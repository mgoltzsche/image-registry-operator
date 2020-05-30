package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	certmgr "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha3"
	certmgrmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	operator "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func createImageRegistry(t *testing.T, ctx *framework.Context) (cr *operator.ImageRegistry) {
	f := framework.Global
	namespace := f.Namespace

	// TODO: also test registry using cert-manager

	// Insert ImageRegistry CR
	//authSecretName := "test-auth-ca"
	//tlsSecretName := "test-tls"
	cr = &operator.ImageRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: namespace,
		},
		Spec: operator.ImageRegistrySpec{
			/*Auth: operator.AuthSpec{
				CA: operator.CertificateSpec{
					SecretName: &authSecretName,
					IssuerRef: &operator.CertIssuerRefSpec{
						Name: "registry-ca-issuer",
						Kind: "Issuer",
					},
				},
			},
			TLS: operator.CertificateSpec{
				SecretName: &tlsSecretName,
				IssuerRef: &operator.CertIssuerRefSpec{
					Name: "registry-ca-issuer",
					Kind: "Issuer",
				},
			},*/
			PersistentVolumeClaim: operator.PersistentVolumeClaimSpec{
				// DeleteClaim=false required initially to avoid https://github.com/kubernetes/minikube/issues/7218
				DeleteClaim: false,
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		},
	}
	err := f.Client.Create(context.TODO(), cr, &framework.CleanupOptions{TestContext: ctx, Timeout: time.Second * 5, RetryInterval: time.Second * 1})
	require.NoError(t, err, "create ImageRegistry")

	/*// Wait for auth certificate to become ready
	waitForCertReady(t, namespace, "imageregistry-"+cr.Name+"-auth-ca", authSecretName, cr.Spec.Auth.CA.IssuerRef)

	// Wait for TLS certificate to become ready
	waitForCertReady(t, namespace, "imageregistry-"+cr.Name+"-tls", tlsSecretName, &operator.CertIssuerRefSpec{
		Name: cr.Name + "-ca-issuer",
		Kind: "Issuer",
	})*/

	// Wait for ImageRegistry to become synced (fail fast)
	waitForRegistrySynced(t, cr)
	// Wait for ImageRegistry to become ready
	waitForRegistryReady(t, cr)

	// Ensure the StatefulSet has been created and is ready
	statefulSet := &appsv1.StatefulSet{}
	key := dynclient.ObjectKey{Name: cr.Name, Namespace: namespace}
	err = f.Client.Get(context.TODO(), key, statefulSet)
	require.NoError(t, err, "get StatefulSet", cr.Name)
	s := statefulSet.Status
	replicas := int32(1)
	ready := statefulSet.Generation == statefulSet.Status.ObservedGeneration &&
		statefulSet.Spec.Replicas != nil &&
		*statefulSet.Spec.Replicas == replicas &&
		s.Replicas == replicas &&
		s.ReadyReplicas == replicas &&
		s.UpdatedReplicas == replicas
	require.True(t, ready, "StatefulSet %s should be ready after ImageRegistry has become ready", cr.Name)

	t.Logf("Updating registry pvc...")
	cr.Spec.PersistentVolumeClaim.DeleteClaim = true
	err = f.Client.Update(context.TODO(), cr)
	require.NoError(t, err, "update ImageRegistry")
	waitForRegistrySynced(t, cr)
	waitForRegistryReady(t, cr)
	return
}

func waitForRegistrySynced(t *testing.T, cr *operator.ImageRegistry) {
	err := WaitForCondition(t, cr, cr.Name, cr.Namespace, 20*time.Second, func() (c []string) {
		if cr.Status.ObservedGeneration != cr.Generation {
			c = append(c, fmt.Sprintf("$.status.observedGeneration == %d (was %v)", cr.Generation, cr.Status.ObservedGeneration))
		}
		if !cr.Status.Conditions.IsTrueFor("Synced") {
			status := "Synced"
			cond := cr.Status.Conditions.GetCondition("Synced")
			if cond != nil && cond.Message != "" {
				status = fmt.Sprintf("Synced{%s}", cond.Message)
			}
			c = append(c, status)
		} else {
			expectedHostname := fmt.Sprintf("%s.%s.svc.cluster.local", cr.Name, cr.Namespace)
			require.Equal(t, expectedHostname, cr.Status.Hostname, "$.status.hostname")
		}
		return
	})
	require.NoError(t, err, "wait for ImageRegistry %s to become synced", cr.Name)
}

func waitForRegistryReady(t *testing.T, cr *operator.ImageRegistry) {
	err := WaitForCondition(t, cr, cr.Name, cr.Namespace, 90*time.Second, func() (c []string) {
		if !cr.Status.Conditions.IsTrueFor("Ready") {
			status := "Ready"
			cond := cr.Status.Conditions.GetCondition("Ready")
			if cond != nil && cond.Message != "" {
				status = fmt.Sprintf("Ready{%s}", cond.Message)
			}
			c = append(c, status)
		}
		return
	})
	require.NoError(t, err, "wait for ImageRegistry %s to become ready", cr.Name)
}

func waitForCertReady(t *testing.T, namespace, certName, expectedSecretName string, expectedIssuer *operator.CertIssuerRefSpec) {
	cert := &certmgr.Certificate{}
	err := WaitForCondition(t, cert, certName, namespace, 15*time.Second, func() (c []string) {
		expectIssuer := fmt.Sprintf("%s/%s", expectedIssuer.Kind, expectedIssuer.Name)
		actualIssuer := fmt.Sprintf("%s/%s", cert.Spec.IssuerRef.Kind, cert.Spec.IssuerRef.Name)
		require.Equal(t, expectIssuer, actualIssuer, "cert %s issuer", certName)
		require.Equal(t, expectedSecretName, cert.Spec.SecretName, "cert %s secret name", certName)

		for _, cond := range cert.Status.Conditions {
			if cond.Type == certmgr.CertificateConditionReady {
				if cond.Status != certmgrmeta.ConditionTrue {
					return []string{cond.Reason + ": " + cond.Message}
				} else {
					return
				}
			}
		}
		return []string{"ready"}
	})
	require.NoError(t, err, "wait for Certificate %s to become ready", certName)
}
