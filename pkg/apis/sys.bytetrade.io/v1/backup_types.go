package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BackupSpec defines the desired state of Backup
type BackupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner *string `json:"owner"`

	TerminusVersion *string `json:"terminusVersion"`

	Size *int64 `json:"size"`

	Phase *string `json:"phase"`

	FailedMessage *string `json:"failedMessage,omitempty"`

	MiddleWarePhase *string `json:"middleWarePhase,omitempty"`

	MiddleWareFailedMessage *string `json:"middleWareFailedMessage,omitempty"`

	ResticPhase *string `json:"resticPhase,omitempty"`

	ResticFailedMessage *string `json:"resticFailedMessage,omitempty"`

	Extra map[string]string `json:"extra,omitempty"`
}

// BackupStatus defines the observed state of Backup
type BackupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+genclient
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced, categories={all}
//+kubebuilder:printcolumn:JSONPath=.spec.owner, name=owner, type=string
//+kubebuilder:printcolumn:JSONPath=.spec.phase, name=phase, type=string
//+kubebuilder:printcolumn:JSONPath=.metadata.creationTimestamp, name=creation, type=date
//+k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Backup is the Schema for the backups API
type Backup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupSpec   `json:"spec,omitempty"`
	Status BackupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
//+k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackupList contains a list of Backup
type BackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Backup{}, &BackupList{})
}
