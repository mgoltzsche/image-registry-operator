package imageregistry

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	certmgr "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha3"
	certmgrmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/certs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcileImageRegistry) reconcileCertificates(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	secretName := authCASecretNameForCR(instance)
	commonName := fmt.Sprintf("%s.%s.svc", instance.Name, instance.Namespace)
	authTokenCaIssuer := instance.Spec.Auth.CA.IssuerRef
	var caCert *certs.KeyPair
	if authTokenCaIssuer != nil {
		caCertCR := &certmgr.Certificate{}
		caCertCR.Name = authCACertNameForCR(instance)
		caCertCR.Namespace = instance.Namespace
		err = r.upsert(instance, caCertCR, reqLogger, func() error {
			caCertCR.Labels = selectorLabelsForCR(instance)
			caCertCR.Spec = certmgr.CertificateSpec{
				IsCA:       true,
				Duration:   &metav1.Duration{Duration: 24 * 365 * 5 * time.Hour},
				CommonName: commonName,
				SecretName: secretName,
				IssuerRef:  toObjectReference(authTokenCaIssuer),
			}
			return nil
		})
	} else if instance.Spec.Auth.CA.SecretName == nil {
		caCert, err = r.reconcileCertSecret(instance, secretName, reqLogger, func() (*certs.KeyPair, error) {
			return certs.NewSelfSignedCAKeyPair(commonName)
		})
		if err != nil {
			return
		}
	}
	if caCert == nil {
		caCert, err = r.loadKeyPair(instance.Namespace, secretName)
		if err != nil {
			return
		}
	}

	return r.reconcileTLSCert(instance, caCert, reqLogger)
}

func (r *ReconcileImageRegistry) loadKeyPair(ns, secretName string) (cert *certs.KeyPair, err error) {
	secret := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: ns}, secret)
	if cert = certFromMap(secret.Data); cert == nil && err == nil {
		err = fmt.Errorf("invalid certificate in Secret %s/%s", ns, secretName)
	}
	return
}

func (r *ReconcileImageRegistry) reconcileTLSCert(instance *registryv1alpha1.ImageRegistry, caCert *certs.KeyPair, reqLogger logr.Logger) (err error) {
	secretName := tlsSecretNameForCR(instance)
	dnsNames := r.dnsNamesForCR(instance)
	tlsIssuer := instance.Spec.TLS.IssuerRef
	if tlsIssuer != nil {
		tlsCertCR := &certmgr.Certificate{}
		tlsCertCR.Name = tlsCertNameForCR(instance)
		tlsCertCR.Namespace = instance.Namespace
		err = r.upsert(instance, tlsCertCR, reqLogger, func() error {
			tlsCertCR.Labels = selectorLabelsForCR(instance)
			tlsCertCR.Spec = certmgr.CertificateSpec{
				IsCA:        false,
				Duration:    &metav1.Duration{Duration: 24 * 90 * time.Hour},
				RenewBefore: &metav1.Duration{Duration: 24 * 20 * time.Hour},
				CommonName:  dnsNames[0],
				DNSNames:    dnsNames,
				SecretName:  secretName,
				IssuerRef:   toObjectReference(tlsIssuer),
			}
			return nil
		})
	} else if instance.Spec.TLS.SecretName == nil {
		_, err = r.reconcileCertSecret(instance, secretName, reqLogger, func() (*certs.KeyPair, error) {
			return certs.NewServerKeyPair(dnsNames, caCert)
		})
	}
	return
}

func (r *ReconcileImageRegistry) dnsNamesForCR(instance *registryv1alpha1.ImageRegistry) []string {
	dnsNames := []string{}
	internalFQN := fmt.Sprintf("%s.%s.svc.cluster.local", instance.Name, instance.Namespace)
	externalFQN := fmt.Sprintf("%s.%s.%s", instance.Name, instance.Namespace, r.dnsZone)
	if externalFQN != internalFQN {
		dnsNames = append(dnsNames, externalFQN)
	}
	return append(dnsNames, internalFQN,
		fmt.Sprintf("%s.%s.svc", instance.Name, instance.Namespace))
}

func toObjectReference(issuer *registryv1alpha1.CertIssuerRefSpec) certmgrmeta.ObjectReference {
	return certmgrmeta.ObjectReference{
		Name: issuer.Name,
		Kind: issuer.Kind,
	}
}
