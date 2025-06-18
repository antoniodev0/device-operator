// mcu_client/src/main.rs

use serde::{Deserialize, Serialize};
use reqwest::StatusCode;

// Definiamo una struct per il corpo della nostra richiesta POST.
// `Serialize` permette di convertirla in JSON.
#[derive(Serialize)]
struct EnrollmentRequest {
    #[serde(rename = "publicKey")]
    public_key: String,
}

// Definiamo una struct per la risposta JSON che ci aspettiamo in caso di successo.
// `Deserialize` permette di convertire il JSON in questa struct.
#[derive(Deserialize, Debug)]
struct EnrollmentResponse {
    #[serde(rename = "deviceUUID")]
    device_uuid: String,
    message: String,
}

// Usiamo il runtime asincrono Tokio.
#[tokio::main]
async fn main() -> Result<(), reqwest::Error> {
    println!("[MCU] Avvio del processo di enrollment...");

    // 1. Definiamo l'indirizzo del nostro Gateway (MPU).
    let gateway_url = "http://localhost:30007/enroll";

    // 2. Generiamo una chiave pubblica a runtime.
    // In un'applicazione reale, useremmo una libreria crittografica (es. `rsa`, `ring`).
    // Per questa simulazione, una stringa unica è sufficiente.
    let public_key = format!("ssh-rsa FAKE-KEY-{}", uuid::Uuid::new_v4().to_string());
    println!("[MCU] Chiave pubblica generata: {:.30}...", public_key);

    // 3. Prepariamo il payload della richiesta.
    let request_payload = EnrollmentRequest {
        public_key: public_key.clone(),
    };

    // 4. Creiamo un client HTTP.
    let client = reqwest::Client::new();
    println!("[MCU] Invio della richiesta di enrollment a {}", gateway_url);

    // 5. Inviamo la richiesta POST asincrona.
    //    - `.post()` imposta il metodo e l'URL.
    //    - `.json()` serializza il nostro `request_payload` in JSON e imposta
    //      automaticamente l'header `Content-Type: application/json`.
    //    - `.send()` invia la richiesta.
    //    - `.await` attende il completamento.
    let response = client.post(gateway_url)
        .json(&request_payload)
        .send()
        .await;

    // 6. Gestiamo la risposta del Gateway.
    match response {
        Ok(res) => {
            // La richiesta è andata a buon fine, ora controlliamo lo status code.
            match res.status() {
                StatusCode::OK => {
                    // Successo (200 OK)! Proviamo a deserializzare il corpo della risposta.
                    match res.json::<EnrollmentResponse>().await {
                        Ok(enrollment_data) => {
                            println!("\n✅ REGISTRAZIONE COMPLETATA CON SUCCESSO!");
                            println!("   - Messaggio dal Gateway: {}", enrollment_data.message);
                            println!("   - UUID del dispositivo assegnato: {}", enrollment_data.device_uuid);
                            println!("[MCU] Salvataggio dell'UUID e conclusione del processo.");
                        }
                        Err(_) => {
                            println!("\n❌ ERRORE: Risposta di successo ricevuta, ma il corpo JSON non è valido.");
                        }
                    }
                }
                status => {
                    // Abbiamo ricevuto uno status code di errore (es. 403 Forbidden).
                    let error_body = res.text().await.unwrap_or_else(|_| "Nessun corpo del messaggio.".to_string());
                    println!("\n❌ REGISTRAZIONE FALLITA!");
                    println!("   - Status Code: {}", status);
                    println!("   - Messaggio di errore dal Gateway: {}", error_body.trim());
                }
            }
        }
        Err(e) => {
            // Errore a livello di rete (es. il Gateway non è raggiungibile).
            println!("\n❌ ERRORE CRITICO: Impossibile connettersi al Gateway.");
            println!("   - Dettagli: {}", e);
            println!("   - Assicurati che il cluster k3d e il Gateway siano in esecuzione e che la porta 30007 sia mappata.");
        }
    }

    Ok(())
}