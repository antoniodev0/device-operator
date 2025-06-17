// in controllers/deviceregistration_controller.go
package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	devicesv1alpha1 "github.com/antonio/device-operator/api/v1alpha1" // Aggiorna con il tuo path corretto
)

const (
	// Definiamo delle costanti per le fasi e il nome del ConfigMap per evitare errori di battitura.
	PhasePending     = "Pending"
	PhaseApproved    = "Approved"
	PhaseRejected    = "Rejected"
	PhaseDeactivated = "Deactivated"
	PairingConfigMapName = "device-pairing-config"
)

// DeviceRegistrationReconciler riconcilia un oggetto DeviceRegistration
type DeviceRegistrationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=devices.example.com,resources=deviceregistrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=devices.example.com,resources=deviceregistrations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=devices.example.com,resources=deviceregistrations/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;watch;list
// ^^^ Abbiamo bisogno dei permessi per leggere i ConfigMap!

func (r *DeviceRegistrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("deviceregistration", req.NamespacedName)

	var dr devicesv1alpha1.DeviceRegistration
	if err := r.Get(ctx, req.NamespacedName, &dr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// === Gestione del ciclo di vita principale ===

	// 1. Gestione deattivazione
	if dr.Spec.Deactivate && dr.Status.Phase == PhaseApproved {
		return r.deactivateDevice(ctx, &dr, logger)
	}

	// 2. Gestione riattivazione
	if !dr.Spec.Deactivate && dr.Status.Phase == PhaseDeactivated {
		return r.reactivateDevice(ctx, &dr, logger)
	}

	// 3. Se la registrazione è già in uno stato terminale (Approved, Rejected), non fare nulla.
	if dr.Status.Phase == PhaseApproved || dr.Status.Phase == PhaseRejected {
		return ctrl.Result{}, nil
	}

	// 4. Gestione della registrazione iniziale (lo stato è vuoto o Pending)
	return r.handleInitialRegistration(ctx, &dr, logger)
}

// handleInitialRegistration gestisce il workflow di una nuova richiesta di registrazione.
func (r *DeviceRegistrationReconciler) handleInitialRegistration(ctx context.Context, dr *devicesv1alpha1.DeviceRegistration, logger logr.Logger) (ctrl.Result, error) {
	// Controlliamo se la modalità di pairing è attiva leggendo il ConfigMap.
	isPairingEnabled, err := r.isPairingModeEnabled(ctx, dr.Namespace)
	if err != nil {
		logger.Error(err, "Impossibile verificare lo stato della modalità di pairing")
		// Se non possiamo leggere il ConfigMap, riproviamo più tardi.
		return ctrl.Result{RequeueAfter: 15 * time.Second}, err
	}

	if !isPairingEnabled {
		logger.Info("Modalità di pairing non attiva. Rifiuto della registrazione.")
		dr.Status.Phase = PhaseRejected
		dr.Status.Message = "Pairing mode is not enabled. The request is rejected."
		if err := r.Status().Update(ctx, dr); err != nil {
			logger.Error(err, "Fallimento nell'aggiornare lo stato a Rejected")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// La modalità di pairing è attiva, procediamo con l'approvazione.
	logger.Info("Modalità di pairing attiva. Approvazione della registrazione in corso.")

	// Genera un UUID univoco per il dispositivo.
	dr.Status.DeviceUUID = uuid.New().String()
	dr.Status.Phase = PhaseApproved
	dr.Status.Message = "Device registered successfully."
	dr.Status.RegistrationTimestamp = time.Now().Format(time.RFC3339)

	if err := r.Status().Update(ctx, dr); err != nil {
		logger.Error(err, "Fallimento nell'aggiornare lo stato a Approved")
		return ctrl.Result{}, err
	}

	logger.Info("Registrazione approvata con successo", "DeviceUUID", dr.Status.DeviceUUID)
	return ctrl.Result{}, nil
}

// isPairingModeEnabled controlla il ConfigMap per vedere se il pairing è abilitato.
func (r *DeviceRegistrationReconciler) isPairingModeEnabled(ctx context.Context, namespace string) (bool, error) {
	pairingConfig := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: PairingConfigMapName, Namespace: namespace}, pairingConfig)

	if err != nil {
		// Se il ConfigMap non esiste, consideriamo il pairing disabilitato per sicurezza.
		if apierrors.IsNotFound(err) {
			r.Log.Info("ConfigMap di pairing non trovato, si presume disabilitato", "configMap", PairingConfigMapName)
			return false, nil
		}
		// Per altri errori, restituiamo l'errore.
		return false, fmt.Errorf("impossibile ottenere il ConfigMap di pairing: %w", err)
	}

	enabled, ok := pairingConfig.Data["enabled"]
	if !ok {
		// Se la chiave 'enabled' non è presente, consideriamo disabilitato.
		r.Log.Info("Chiave 'enabled' non trovata nel ConfigMap, si presume disabilitato", "configMap", PairingConfigMapName)
		return false, nil
	}

	return enabled == "true", nil
}

// deactivateDevice gestisce la logica per deattivare un dispositivo.
func (r *DeviceRegistrationReconciler) deactivateDevice(ctx context.Context, dr *devicesv1alpha1.DeviceRegistration, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Deattivazione del dispositivo in corso...")
	dr.Status.Phase = PhaseDeactivated
	dr.Status.Message = "Device has been deactivated by an administrator."
	// Manteniamo il timestamp di registrazione originale.
	if err := r.Status().Update(ctx, dr); err != nil {
		logger.Error(err, "Fallimento nell'aggiornare lo stato a Deactivated")
		return ctrl.Result{}, err
	}
	logger.Info("Dispositivo deattivato con successo")
	return ctrl.Result{}, nil
}

// reactivateDevice gestisce la logica per riattivare un dispositivo.
func (r *DeviceRegistrationReconciler) reactivateDevice(ctx context.Context, dr *devicesv1alpha1.DeviceRegistration, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Riattivazione del dispositivo in corso...")
	dr.Status.Phase = PhaseApproved
	dr.Status.Message = "Device has been reactivated."
	// Potremmo decidere di aggiornare o meno il timestamp. Lasciamolo così per ora.
	if err := r.Status().Update(ctx, dr); err != nil {
		logger.Error(err, "Fallimento nell'aggiornare lo stato ad Approved (riattivazione)")
		return ctrl.Result{}, err
	}
	logger.Info("Dispositivo riattivato con successo")
	return ctrl.Result{}, nil
}

func (r *DeviceRegistrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&devicesv1alpha1.DeviceRegistration{}).
		// Aggiungiamo un watch sul ConfigMap.
		// Se il ConfigMap cambia, vogliamo riconciliare TUTTE le risorse in stato Pending.
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}