package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImagePullSecret is the Schema for the imagepullsecrets API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=imagepullsecrets,scope=Namespaced
type ImagePullSecret struct {
	ImageSecret `json:",inline"`
}

func (_ *ImagePullSecret) GetRegistryAccessMode() ImageSecretType {
	return TypePull
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImagePullSecretList contains a list of ImagePullSecret
type ImagePullSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImagePullSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImagePullSecret{}, &ImagePullSecretList{})
}
