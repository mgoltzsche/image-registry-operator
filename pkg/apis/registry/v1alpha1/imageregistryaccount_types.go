package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageRegistryAccountSpec defines the desired state of ImageRegistryAccount
type ImageRegistryAccountSpec struct {
	// bcrypt hashed password
	Password string `json:"password"`
	// Labels to match against authorization rules
	Labels map[string][]string `json:"labels,omitempty"`
	TTL    *metav1.Duration    `json:"ttl,omitempty"`
}

// ImageRegistryAccountStatus defines the observed state of ImageRegistryAccount
type ImageRegistryAccountStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageRegistryAccount is the Schema for the imageregistryaccounts API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=imageregistryaccounts,scope=Namespaced
type ImageRegistryAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ImageRegistryAccountSpec `json:"spec,omitempty"`
}

func (a *ImageRegistryAccount) Expired() bool {
	if a.Spec.TTL == nil {
		return false
	}
	return time.Now().After(a.CreationTimestamp.Add(a.Spec.TTL.Duration))
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageRegistryAccountList contains a list of ImageRegistryAccount
type ImageRegistryAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageRegistryAccount `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageRegistryAccount{}, &ImageRegistryAccountList{})
}
