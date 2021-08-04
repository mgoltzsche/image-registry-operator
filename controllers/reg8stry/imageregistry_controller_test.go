package reg8stry

import (
	"context"
	"time"

	"github.com/mgoltzsche/reg8stry"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var (
	fakeProvisionerDeleteOnPodTermination = "delete-on-pod-termination.fake.provisioner"
	fakeProvisionerNoDeletion             = "ignore-terminating-pod.fake.provisioner"
)

var _ = Describe("PodController", func() {
	Describe("completed pod", func() {
		It("should annotate and delete PVC of matching provisioner when auto deletion is enabled", func() {
			// TODO: configure fake provisioner within reconciler
			pvcMatching := createPVC("matching-pvc", fakeProvisionerDeleteOnPodTermination)
			pvcOther := createPVC("other-pvc", "other."+fakeProvisionerDeleteOnPodTermination)
			podCompleted := createPod("completed-matching-provisioner", []string{"true"}, corev1.RestartPolicyNever, pvcOther, pvcMatching)
			setPodPhase(podCompleted, corev1.PodSucceeded)
			verify(pvcMatching, hasBeenDeleted(pvcMatching))
		})
	})
})
