package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type BackupPolicy struct {
	Name string `json:"name"`

	SnapshotFrequency string `json:"snapshotFrequency"`

	TimesOfDay string `json:"timesOfDay"`

	DayOfWeek int `json:"dayOfWeek"`

	Enabled bool `json:"enabled"`
}

// BackupConfigSpec defines the desired state of BackupConfig
type BackupConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Provider string `json:"provider"`

	Region string `json:"region"`

	Prefix string `json:"prefix,omitempty"`

	Bucket string `json:"bucket"`

	S3Url string `json:"s3Url,omitempty"`

	AccessKey string `json:"accessKey,omitempty"`

	SecretKey string `json:"secretKey,omitempty"`

	Plugins []string `json:"plugins"`

	Owner string `json:"owner,omitempty"`

	Location string `json:"location"`

	StorageLocation string `json:"storageLocation"`

	BackupPolicy *BackupPolicy `json:"backupPolicy,omitempty"`

	RepositoryPassword string `json:"repositoryPassword,omitempty"`

	Extra map[string]string `json:"extra,omitempty"`
}

// BackupConfigStatus defines the observed state of BackupConfig
type BackupConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	State      string      `json:"state"`
	UpdateTime metav1.Time `json:"updateTime"`
}

//+genclient
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced, shortName={bc}, categories={all}
//+kubebuilder:printcolumn:JSONPath=.spec.provider, name=provider, type=string
//+kubebuilder:printcolumn:JSONPath=.spec.region, name=region, type=string
//+kubebuilder:printcolumn:JSONPath=.spec.bucket, name=bucket, type=string
//+kubebuilder:printcolumn:JSONPath=.spec.prefix, name=prefix, type=string
//+kubebuilder:printcolumn:JSONPath=.spec.owner, name=owner, type=string
//+kubebuilder:printcolumn:JSONPath=.spec.location, name=location, type=string
//+kubebuilder:printcolumn:JSONPath=.spec.storageLocation, name=storageLocation, type=string
//+kubebuilder:printcolumn:JSONPath=.metadata.creationTimestamp, name=age, type=date
//+kubebuilder:printcolumn:JSONPath=.status.updateTime, name=updateTime, type=date
//+k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackupConfig is the Schema for the backupconfigs API
type BackupConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupConfigSpec   `json:"spec,omitempty"`
	Status BackupConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
//+k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackupConfigList contains a list of BackupConfig
type BackupConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupConfig{}, &BackupConfigList{})
}
