package imageregistry

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	certmgr "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha3"
	certmgrmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/certs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	labels = map[string]string{"name": "image-registry-operator"}
)

func (r *ReconcileImageRegistry) reconcileTokenCert(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	secretName := authCASecretNameForCR(instance)
	commonName := fmt.Sprintf("%s.%s.svc", instance.Name, instance.Namespace)
	labels := selectorLabelsForCR(instance)
	authTokenCaIssuer := instance.Spec.Auth.CA.IssuerRef
	if authTokenCaIssuer != nil {
		caCertCR := &certmgr.Certificate{}
		caCertCR.Name = authCACertNameForCR(instance)
		caCertCR.Namespace = instance.Namespace
		err = r.upsert(instance, caCertCR, reqLogger, func() error {
			caCertCR.Labels = labels
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
		key := types.NamespacedName{Name: secretName, Namespace: instance.Namespace}
		_, err = r.certManager.RenewCACertSecret(key, instance, labels, commonName)
	}
	return
}

func (r *ReconcileImageRegistry) reconcileTLSCert(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	secretName := tlsSecretNameForCR(instance)
	dnsNames := r.dnsNamesForCR(instance)
	labels := selectorLabelsForCR(instance)
	tlsIssuer := instance.Spec.TLS.IssuerRef
	if tlsIssuer != nil {
		tlsCertCR := &certmgr.Certificate{}
		tlsCertCR.Name = tlsCertNameForCR(instance)
		tlsCertCR.Namespace = instance.Namespace
		err = r.upsert(instance, tlsCertCR, reqLogger, func() error {
			tlsCertCR.Labels = labels
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
		ca, e := r.certManager.RenewCACertSecret(r.rootCASecretName, nil, labels, "image-registry-operator.caroot.local")
		if e != nil && (errors.Unwrap(e) != certs.ErrUnmanagedValidSecretExists || ca == nil) {
			return e
		}
		key := types.NamespacedName{Name: secretName, Namespace: instance.Namespace}
		_, err = r.certManager.RenewServerCertSecret(key, instance, labels, dnsNames, ca)
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
