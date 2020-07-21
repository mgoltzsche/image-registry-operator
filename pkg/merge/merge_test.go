package merge

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestAddContainer(t *testing.T) {
	pod := &corev1.PodSpec{}
	image1 := "myappimg:latest"
	image2 := "myappimg:1.0.0"
	AddContainer(pod, corev1.Container{Image: image1})
	require.Equal(t, 1, len(pod.Containers), "len(pod.Containers)")
	require.Equal(t, image1, pod.Containers[0].Image)
	AddContainer(pod, corev1.Container{Image: image2})
	require.Equal(t, 1, len(pod.Containers), "len(pod.Containers)")
	require.Equal(t, image2, pod.Containers[0].Image)
}
