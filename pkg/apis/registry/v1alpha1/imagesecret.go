package v1alpha1

import (
	"github.com/operator-framework/operator-sdk/pkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	TypePull ImageSecretType = "pull"
	TypePush ImageSecretType = "push"
)

type ImageSecretType string

type ImageSecretInterface interface {
	runtime.Object
	metav1.Object
	GetRegistryRef() *ImageRegistryRef
	GetStatus() *ImageSecretStatus
}

type ImageSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageSecretSpec   `json:"spec,omitempty"`
	Status ImageSecretStatus `json:"status,omitempty"`
}

func (s *ImageSecret) GetRegistryRef() *ImageRegistryRef {
	return s.Spec.RegistryRef
}

func (s *ImageSecret) GetStatus() *ImageSecretStatus {
	return &s.Status
}

// ImageSecretSpec defines the desired state of ImagePushSecret/ImagePullSecret
type ImageSecretSpec struct {
	RegistryRef *ImageRegistryRef `json:"registryRef,omitempty"`
}

// ImageRegistryRef refers to an ImageRegistry
type ImageRegistryRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ImageSecretStatus defines the observed state of ImagePullSecret
type ImageSecretStatus struct {
	// The currently active bcrypt encoded passwords - should be two.
	Passwords []string `json:"passwords,omitempty"`
	// Password rotation amount.
	Rotation uint64 `json:"rotation,omitempty"`
	// Date on which the latest password has been generated.
	RotationDate *metav1.Time `json:"rotationDate,omitempty"`
	// Conditions represent the latest available observations of an object's state
	Conditions status.Conditions `json:"conditions,omitempty"`
}
