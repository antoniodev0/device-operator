/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeviceRegistrationSpec definisce lo stato voluto di una richiesta di registrazione.
// Questa risorsa è tipicamente creata da un gateway quando un dispositivo cerca di connettersi.
type DeviceRegistrationSpec struct {
	// PublicKey del dispositivo che richiede la registrazione, in formato PEM o simile.
	// Questo campo è obbligatorio per una richiesta di registrazione.
	// +kubebuilder:validation:Required
	PublicKey string `json:"publicKey"`

	// Deactivate, se impostato a true, avvia il workflow di deattivazione per un dispositivo già approvato.
	// L'amministratore può impostare questo flag per disabilitare temporaneamente un dispositivo.
	// +optional
	Deactivate bool `json:"deactivate,omitempty"`
}

// DeviceRegistrationStatus definisce lo stato osservato di DeviceRegistration.
type DeviceRegistrationStatus struct {
	// Phase indica la fase corrente del ciclo di vita della registrazione.
	// Valori possibili: Pending, Approved, Rejected, Deactivated.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message fornisce dettagli leggibili sull'esito della registrazione o dello stato corrente.
	// +optional
	Message string `json:"message,omitempty"`

	// RegistrationTimestamp è il timestamp di quando la registrazione è stata approvata.
	// +optional
	RegistrationTimestamp string `json:"registrationTimestamp,omitempty"` // Formato RFC3339

	// DeviceUUID è l'identificatore univoco assegnato al dispositivo dall'operatore
	// dopo che la registrazione è stata approvata. Questo è l'ID ufficiale del dispositivo nel sistema.
	// +optional
	DeviceUUID string `json:"deviceUUID,omitempty"`

	// Conditions fornisce una lista di condizioni che descrivono lo stato corrente della risorsa.
	// Utile per una diagnostica dettagliata.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase",description="The current status of the registration"
// +kubebuilder:printcolumn:name="UUID",type="string",JSONPath=".status.deviceUUID",description="The UUID assigned to the device"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// DeviceRegistration è la risorsa Custom per una richiesta di registrazione di un dispositivo.
type DeviceRegistration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceRegistrationSpec   `json:"spec,omitempty"`
	Status DeviceRegistrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// DeviceRegistrationList contiene una lista di DeviceRegistration.
type DeviceRegistrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeviceRegistration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeviceRegistration{}, &DeviceRegistrationList{})
}
