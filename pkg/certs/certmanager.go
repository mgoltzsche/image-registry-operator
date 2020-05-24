package certs

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	secretKeyCACrt  = "ca.crt"
	secretKeyTLSCrt = "tls.crt"
	secretKeyTLSKey = "tls.key"
)

type CertManager struct {
	client client.Client
}

func NewCertManager(client client.Client) *CertManager {
	return &CertManager{client}
}

func (r *CertManager) RenewCACertSecret(key types.NamespacedName, labels map[string]string, commonName string) (cert *KeyPair, err error) {
	secret := &corev1.Secret{}
	secret.Name = key.Name
	secret.Namespace = key.Namespace
	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.client, secret, func() (e error) {
		secret.Labels = labels
		secret.Type = corev1.SecretTypeTLS
		cert = certFromMap(secret.Data)
		if cert == nil || cert.NeedsRenewal() || !cert.IsCA() {
			cert, e = NewSelfSignedCAKeyPair(commonName)
			if e != nil {
				return e
			}
			secret.Data = certToMap(cert)
		}
		return
	})
	if err != nil {
		err = fmt.Errorf("upsert CA root secret %s/%s: %w", secret.Namespace, secret.Name, err)
	}
	return
}

func (r *CertManager) RenewServerCertSecret(key types.NamespacedName, labels map[string]string, dnsNames []string, ca *KeyPair) (cert *KeyPair, err error) {
	secret := &corev1.Secret{}
	secret.Name = key.Name
	secret.Namespace = key.Namespace
	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.client, secret, func() (e error) {
		secret.Labels = labels
		secret.Type = corev1.SecretTypeTLS
		cert = certFromMap(secret.Data)
		if cert == nil || cert.NeedsRenewal() || cert.IsCA() || !equalNames(cert.DNSNames(), dnsNames) {
			cert, e = NewServerKeyPair(dnsNames, ca)
			secret.Data = certToMap(cert)
		}
		return
	})
	if err != nil {
		err = fmt.Errorf("upsert certificate secret %s/%s: %w", secret.Namespace, secret.Name, err)
	}
	return
}

func (r *CertManager) KeyPair(name types.NamespacedName) (cert *KeyPair, err error) {
	secret := &corev1.Secret{}
	err = r.client.Get(context.TODO(), name, secret)
	if cert = certFromMap(secret.Data); cert == nil && err == nil {
		return nil, fmt.Errorf("invalid certificate in Secret %s/%s", name.Namespace, name.Name)
	}
	return
}

func certFromMap(m map[string][]byte) (c *KeyPair) {
	if m != nil {
		c, _ = X509KeyPair(m[secretKeyTLSKey], m[secretKeyTLSCrt], m[secretKeyCACrt])
	}
	return
}

func certToMap(cert *KeyPair) map[string][]byte {
	return map[string][]byte{
		secretKeyCACrt:  cert.CACertPEM(),
		secretKeyTLSCrt: cert.CertPEM(),
		secretKeyTLSKey: cert.KeyPEM(),
	}
}

func equalNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
