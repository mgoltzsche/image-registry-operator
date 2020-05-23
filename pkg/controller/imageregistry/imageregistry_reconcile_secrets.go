package imageregistry

import (
	"github.com/go-logr/logr"
	registryapi "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/certs"
	corev1 "k8s.io/api/core/v1"
)

const (
	secretKeyCACrt  = "ca.crt"
	secretKeyTLSCrt = "tls.crt"
	secretKeyTLSKey = "tls.key"
)

type certFactory func() (*certs.KeyPair, error)

func (r *ReconcileImageRegistry) reconcileCertSecret(instance *registryapi.ImageRegistry, name string, reqLogger logr.Logger, factory certFactory) (cert *certs.KeyPair, err error) {
	labels := selectorLabelsForCR(instance)
	caCertSecret := &corev1.Secret{}
	caCertSecret.Name = name
	caCertSecret.Namespace = instance.Namespace
	err = r.upsert(instance, caCertSecret, reqLogger, func() (e error) {
		caCertSecret.Labels = labels
		caCertSecret.Type = corev1.SecretTypeTLS
		cert = certFromMap(caCertSecret.Data)
		if cert == nil || cert.NeedsRenewal() {
			cert, e = factory()
			caCertSecret.Data = certToMap(cert)
		}
		return
	})
	return
}

func certFromMap(m map[string][]byte) (c *certs.KeyPair) {
	if m != nil {
		c, _ = certs.X509KeyPair(m[secretKeyTLSKey], m[secretKeyTLSCrt], m[secretKeyCACrt])
	}
	return
}

func certToMap(cert *certs.KeyPair) map[string][]byte {
	return map[string][]byte{
		secretKeyCACrt:  cert.CACertPEM(),
		secretKeyTLSCrt: cert.CertPEM(),
		secretKeyTLSKey: cert.KeyPEM(),
	}
}
