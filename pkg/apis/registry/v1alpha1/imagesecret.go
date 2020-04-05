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

type ImageSecret interface {
	runtime.Object
	metav1.Object
	GetStatus() *ImageSecretStatus
}

// ImageSecretStatus defines the observed state of ImagePullSecret
type ImageSecretStatus struct {
	// The currently active bcrypt encoded passwords - should be two.
	Passwords []string `json:"passwords,omitempty"`
	// Password rotation amount.
	Rotation uint64 `json:"rotation,omitempty"`
	// Date on which the latest password has been generated.
	RotationDate metav1.Time `json:"rotationDate,omitempty"`
	// Conditions represent the latest available observations of an object's state
	Conditions status.Conditions `json:"conditions,omitempty"`
}
