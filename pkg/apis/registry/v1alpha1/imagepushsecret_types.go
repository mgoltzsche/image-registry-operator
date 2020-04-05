package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ImagePushSecretSpec defines the desired state of ImagePushSecret
type ImagePushSecretSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImagePushSecret is the Schema for the imagepushsecrets API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=imagepushsecrets,scope=Namespaced
type ImagePushSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImagePushSecretSpec `json:"spec,omitempty"`
	Status ImageSecretStatus   `json:"status,omitempty"`
}

func (s *ImagePushSecret) GetStatus() *ImageSecretStatus {
	return &s.Status
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImagePushSecretList contains a list of ImagePushSecret
type ImagePushSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImagePushSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImagePushSecret{}, &ImagePushSecretList{})
}
