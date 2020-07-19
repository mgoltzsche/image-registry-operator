package v1alpha1

import (
	"github.com/operator-framework/operator-sdk/pkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ReasonMissingSecret    = status.ConditionReason("MissingSecret")
	ReasonInvalidSecret    = status.ConditionReason("InvalidSecret")
	ReasonFailedUpdate     = status.ConditionReason("FailedUpdate")
	ReasonPending          = status.ConditionReason("Pending")
	SecretKeyMakisuYAML    = "makisu.yaml"
	SecretKeyRedis         = "redis"
	SecretKeyRedisPassword = "redis_password"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ImageBuildEnvSpec defines the desired state of ImageBuildEnv
type ImageBuildEnvSpec struct {
	Redis   bool             `json:"redis,omitempty"`
	Secrets []ImageSecretRef `json:"secrets,omitempty"`
}

type ImageSecretRef struct {
	SecretName string `json:"secretName"`
}

// ImageBuildEnvStatus defines the observed state of ImageBuildEnv
type ImageBuildEnvStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions represent the latest available observations of an object's state
	Conditions status.Conditions `json:"conditions,omitempty"`
	// SecretRefs lists the watched input secrets
	SecretRefs []string `json:"secretRefs,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageBuildEnv is the Schema for the imagebuildenvs API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=imagebuildenvs,scope=Namespaced
type ImageBuildEnv struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageBuildEnvSpec   `json:"spec,omitempty"`
	Status ImageBuildEnvStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImageBuildEnvList contains a list of ImageBuildEnv
type ImageBuildEnvList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageBuildEnv `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageBuildEnv{}, &ImageBuildEnvList{})
}
