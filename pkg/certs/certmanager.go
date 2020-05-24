package certs

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	ErrUnmanagedValidSecretExists   = errors.New("refuse to overwrite valid unmanaged cert secret")
	ErrUnmanagedInvalidSecretExists = errors.New("refuse to overwrite invalid/renewable unmanaged cert secret")
)

const (
	secretKeyCACrt  = "ca.crt"
	secretKeyTLSCrt = "tls.crt"
	secretKeyTLSKey = "tls.key"
	labelManagedBy  = "app.kubernetes.io/managed-by"
	operatorName    = "image-registry-operator"
)

type CertManager struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewCertManager(client client.Client, scheme *runtime.Scheme) *CertManager {
	return &CertManager{client, scheme}
}

func (r *CertManager) RenewCACertSecret(key types.NamespacedName, owner metav1.Object, labels map[string]string, commonName string) (cert *KeyPair, err error) {
	return r.renewCertSecret(key, owner, labels, condCACert, func() (*KeyPair, error) {
		return NewSelfSignedCAKeyPair(commonName)
	})
}

func (r *CertManager) RenewServerCertSecret(key types.NamespacedName, owner metav1.Object, labels map[string]string, dnsNames []string, ca *KeyPair) (cert *KeyPair, err error) {
	return r.renewCertSecret(key, owner, labels, condServerCert(dnsNames), func() (*KeyPair, error) {
		return NewServerKeyPair(dnsNames, ca)
	})
}

func (r *CertManager) renewCertSecret(key types.NamespacedName, owner metav1.Object, labels map[string]string, cond func(*KeyPair) bool, factory func() (*KeyPair, error)) (cert *KeyPair, err error) {
	secret := &corev1.Secret{}
	secret.Name = key.Name
	secret.Namespace = key.Namespace
	secretLabels := map[string]string{}
	for k, v := range labels {
		secretLabels[k] = v
	}
	secretLabels[labelManagedBy] = operatorName
	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.client, secret, func() (e error) {
		cert = certFromMap(secret.Data)
		if owner != nil {
			if e = controllerutil.SetControllerReference(owner, secret, r.scheme); e != nil {
				return
			}
		}
		if secret.UID != "" && (secret.Labels == nil || secret.Labels[labelManagedBy] != operatorName) {
			if cert == nil || cert.NeedsRenewal() {
				return ErrUnmanagedInvalidSecretExists
			} else {
				return ErrUnmanagedValidSecretExists
			}
		}
		secret.Labels = secretLabels
		secret.Type = corev1.SecretTypeTLS
		if cert == nil || cert.NeedsRenewal() || cond(cert) {
			cert, e = factory()
			secret.Data = certToMap(cert)
		}
		return
	})
	if err != nil {
		err = fmt.Errorf("upsert certificate secret %s/%s: %w", secret.Namespace, secret.Name, err)
	}
	return
}

func condServerCert(dnsNames []string) func(*KeyPair) bool {
	return func(cert *KeyPair) bool {
		return cert.IsCA() || !equalNames(cert.DNSNames(), dnsNames)
	}
}

func condCACert(cert *KeyPair) bool {
	return !cert.IsCA()
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
