// gateway/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	// Librerie necessarie
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Definiamo le strutture dei dati JSON per le richieste e le risposte.

// EnrollmentRequest è ciò che il dispositivo invia al Gateway.
// Contiene solo la sua chiave pubblica.
type EnrollmentRequest struct {
	PublicKey string `json:"publicKey"`
}

// EnrollmentResponse è ciò che il Gateway restituisce al dispositivo se la registrazione ha successo.
// Contiene l'UUID assegnato dall'operatore.
type EnrollmentResponse struct {
	DeviceUUID string `json:"deviceUUID"`
	Message    string `json:"message"`
}

// gatewayHandler contiene il client Kubernetes e altre informazioni necessarie.
type gatewayHandler struct {
	kubeClient dynamic.Interface // Un client per interagire con le risorse Kubernetes
	namespace  string            // Il namespace in cui operare
}

// newGatewayHandler è una funzione "costruttore" che crea e inizializza il nostro gestore.
func newGatewayHandler() (*gatewayHandler, error) {
	log.Println("Inizializzazione del client Kubernetes...")

	// rest.InClusterConfig() è la magia che permette a un'applicazione
	// di trovare e autenticarsi all'API di Kubernetes quando è in esecuzione
	// all'interno di un Pod nel cluster.
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("impossibile ottenere la configurazione del cluster: %w. Assicurati di eseguire questo codice all'interno di un cluster Kubernetes", err)
	}

	// Creiamo un client "dinamico". Questo tipo di client è perfetto per lavorare
	// con le Custom Resources (CRD) perché non richiede di importare il loro codice Go.
	// Può lavorare con qualsiasi risorsa, basta conoscerne il nome e la struttura.
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("impossibile creare il client dinamico: %w", err)
	}
	log.Println("Client Kubernetes creato con successo.")

	// Per sapere in quale namespace creare le risorse, leggiamo una variabile d'ambiente
	// che verrà iniettata nel Pod dal file deployment.yaml.
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default" // Se la variabile non è impostata, usiamo 'default' come fallback.
		log.Printf("Variabile d'ambiente POD_NAMESPACE non trovata. Uso il namespace di fallback: '%s'\n", namespace)
	} else {
		log.Printf("Opero nel namespace: '%s'\n", namespace)
	}

	return &gatewayHandler{
		kubeClient: dynamicClient,
		namespace:  namespace,
	}, nil
}

// ServeHTTP è il metodo che viene chiamato per ogni richiesta HTTP in arrivo all'endpoint /enroll.
func (h *gatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("Ricevuta richiesta: %s %s", r.Method, r.URL.Path)
	
	// Accettiamo solo richieste POST.
	if r.Method != http.MethodPost {
		http.Error(w, "Metodo non consentito. Usare POST.", http.StatusMethodNotAllowed)
		return
	}

	// Decodifichiamo il corpo JSON della richiesta nella nostra struct EnrollmentRequest.
	var req EnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Corpo della richiesta JSON non valido.", http.StatusBadRequest)
		return
	}

	// Validiamo che la chiave pubblica sia stata fornita.
	if req.PublicKey == "" {
		http.Error(w, "Il campo 'publicKey' è obbligatorio.", http.StatusBadRequest)
		return
	}
	log.Printf("Richiesta di enrollment valida ricevuta per la chiave pubblica: %.20s...", req.PublicKey)

	// Creiamo la risorsa DeviceRegistration nel cluster Kubernetes.
	drName, err := h.createDeviceRegistrationResource(r.Context(), req.PublicKey)
	if err != nil {
		log.Printf("ERRORE: Impossibile creare la risorsa DeviceRegistration: %v", err)
		http.Error(w, "Errore interno del server durante la creazione della richiesta.", http.StatusInternalServerError)
		return
	}

	log.Printf("Risorsa DeviceRegistration '%s' creata. In attesa di elaborazione da parte dell'operatore...", drName)

	// Ora inizia la parte cruciale: aspettiamo che l'operatore faccia il suo lavoro.
	// Facciamo "polling", cioè controlliamo lo stato della risorsa a intervalli regolari.
	uuid, err := h.waitForApproval(r.Context(), drName)
	if err != nil {
		// Se c'è un errore (es. timeout o registrazione rifiutata), lo registriamo e rispondiamo con un errore.
		log.Printf("ERRORE: La registrazione per '%s' è fallita: %v", drName, err)
		http.Error(w, fmt.Sprintf("Registrazione fallita: %v", err), http.StatusForbidden) // 403 Forbidden è un buon codice per un rifiuto.
		return
	}

	log.Printf("SUCCESSO: Registrazione per '%s' approvata. UUID assegnato: %s", drName, uuid)

	// Se tutto è andato bene, inviamo la risposta di successo al dispositivo.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(EnrollmentResponse{
		DeviceUUID: uuid,
		Message:    "Dispositivo registrato con successo.",
	})
}

// createDeviceRegistrationResource crea l'oggetto CRD nel cluster.
func (h *gatewayHandler) createDeviceRegistrationResource(ctx context.Context, publicKey string) (string, error) {
	// Definiamo lo "schema" della nostra risorsa Custom (GVR: Group, Version, Resource).
	// Questi valori devono corrispondere esattamente a quelli nella tua CRD.
	deviceRegistrationGVR := schema.GroupVersionResource{
		Group:    "devices.example.com",
		Version:  "v1alpha1",
		Resource: "deviceregistrations",
	}

	// Per evitare conflitti di nomi, generiamo un nome univoco per ogni richiesta di registrazione.
	resourceName := "dev-reg-" + uuid.New().String()[:8]

	// Costruiamo l'oggetto risorsa usando una mappa "unstructured".
	// Questo ci permette di creare qualsiasi risorsa senza bisogno del suo tipo Go specifico.
	drObject := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "devices.example.com/v1alpha1",
			"kind":       "DeviceRegistration",
			"metadata": map[string]interface{}{
				"name":      resourceName,
				"namespace": h.namespace,
			},
			"spec": map[string]interface{}{
				"publicKey": publicKey,
			},
		},
	}

	// Usiamo il client dinamico per creare la risorsa nel cluster.
	_, err := h.kubeClient.Resource(deviceRegistrationGVR).Namespace(h.namespace).Create(ctx, drObject, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return resourceName, nil
}

// waitForApproval controlla periodicamente lo stato della CR finché non è approvata o rifiutata.
func (h *gatewayHandler) waitForApproval(ctx context.Context, name string) (string, error) {
	var deviceUUID string

	// Definiamo un timeout. Se l'operatore non processa la richiesta entro 2 minuti, la richiesta fallisce.
	// Questo evita che il gateway resti in attesa all'infinito.
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	gvr := schema.GroupVersionResource{Group: "devices.example.com", Version: "v1alpha1", Resource: "deviceregistrations"}

	// wait.PollImmediateUntilWithContext è una funzione di utilità di Kubernetes che esegue una funzione
	// a intervalli regolari (ogni 2 secondi) fino a quando non restituisce 'true' o un errore, o fino al timeout.
	err := wait.PollImmediateUntilWithContext(timeoutCtx, 2*time.Second, func(ctx context.Context) (bool, error) {
		// Ad ogni tentativo, otteniamo la versione più recente della nostra risorsa.
		res, err := h.kubeClient.Resource(gvr).Namespace(h.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err // Errore nel recuperare la risorsa, il polling si fermerà e restituirà questo errore.
		}

		// Estraiamo il campo 'status' dalla risorsa.
		status, found, err := unstructured.NestedMap(res.Object, "status")
		if err != nil || !found {
			log.Printf("In attesa che l'operatore imposti lo stato per '%s'...", name)
			return false, nil // Lo status non è ancora pronto, continua il polling.
		}

		// Controlliamo la 'phase' all'interno dello status.
		phase, _ := status["phase"].(string)
		switch phase {
		case "Approved":
			uuid, ok := status["deviceUUID"].(string)
			if ok && uuid != "" {
				deviceUUID = uuid
				return true, nil // Fatto! La fase è Approved e abbiamo l'UUID. Smettiamo di fare polling.
			}
		case "Rejected":
			// Se la fase è Rejected, è un errore terminale.
			message, _ := status["message"].(string)
			return false, fmt.Errorf("registrazione rifiutata: %s", message) // Smettiamo di fare polling e restituiamo un errore.
		}

		// Se la fase non è né Approved né Rejected (es. è vuota o 'Pending'), continuiamo il polling.
		return false, nil
	})

	if err != nil {
		return "", err // Se il polling è fallito (per timeout o perché è stato rifiutato), restituiamo l'errore.
	}

	return deviceUUID, nil
}


// La funzione main è il punto di ingresso della nostra applicazione.
func main() {
	// Creiamo il nostro gestore di richieste. Se fallisce, il programma si ferma.
	handler, err := newGatewayHandler()
	if err != nil {
		log.Fatalf("ERRORE FATALE: Impossibile inizializzare il gateway: %v", err)
	}

	// Registriamo il nostro gestore per l'endpoint "/enroll".
	http.Handle("/enroll", handler)

	// Avviamo il server web sulla porta 8080.
	log.Println("Gateway in ascolto sulla porta :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("ERRORE FATALE: Impossibile avviare il server HTTP: %v", err)
	}
}