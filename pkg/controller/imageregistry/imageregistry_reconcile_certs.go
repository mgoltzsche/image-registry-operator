package imageregistry

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	certmgr "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha3"
	certmgrmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *ReconcileImageRegistry) reconcileCaCertAndIssuer(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	authTokenCaIssuer := r.authCaIssuerRefForCR(instance)
	if authTokenCaIssuer != nil {
		labels := selectorLabelsForCR(instance)
		caCertCR := &certmgr.Certificate{}
		caCertCR.Name = caSecretNameForCR(instance)
		caCertCR.Namespace = instance.Namespace
		err = r.upsert(instance, caCertCR, reqLogger, func() error {
			caCertCR.Labels = labels
			caCertCR.Spec = certmgr.CertificateSpec{
				IsCA:       true,
				Duration:   &metav1.Duration{Duration: 24 * 365 * 5 * time.Hour},
				CommonName: fmt.Sprintf("%s.%s.svc", instance.Name, instance.Namespace),
				SecretName: caCertCR.Name,
				IssuerRef:  toObjectReference(authTokenCaIssuer),
			}
			return nil
		})
		if err != nil {
			return
		}
		issuerCR := &certmgr.Issuer{}
		issuerCR.Name = caIssuerNameForCR(instance)
		issuerCR.Namespace = instance.Namespace
		err = r.upsert(instance, issuerCR, reqLogger, func() error {
			issuerCR.Labels = labels
			issuerCR.Spec = certmgr.IssuerSpec{
				IssuerConfig: certmgr.IssuerConfig{
					CA: &certmgr.CAIssuer{SecretName: caCertCR.Name},
				},
			}
			return nil
		})
	}
	return
}

func (r *ReconcileImageRegistry) reconcileTlsCert(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	tlsIssuer := r.tlsIssuerRefForCR(instance)
	if tlsIssuer != nil {
		dnsNames := r.dnsNamesForCR(instance)
		tlsCertCR := &certmgr.Certificate{}
		tlsCertCR.Name = TLSSecretNameForCR(instance)
		tlsCertCR.Namespace = instance.Namespace
		err = r.upsert(instance, tlsCertCR, reqLogger, func() error {
			tlsCertCR.Labels = selectorLabelsForCR(instance)
			tlsCertCR.Spec = certmgr.CertificateSpec{
				IsCA:        false,
				Duration:    &metav1.Duration{Duration: 24 * 90 * time.Hour},
				RenewBefore: &metav1.Duration{Duration: 24 * 20 * time.Hour},
				CommonName:  dnsNames[0],
				DNSNames:    dnsNames,
				SecretName:  tlsCertCR.Name,
				IssuerRef:   toObjectReference(tlsIssuer),
			}
			return nil
		})
	}
	return err
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

func (r *ReconcileImageRegistry) tlsIssuerRefForCR(instance *registryv1alpha1.ImageRegistry) (issuer *registryv1alpha1.CertIssuerRefSpec) {
	issuer = instance.Spec.TLS.IssuerRef
	authCaIssuer := r.authCaIssuerRefForCR(instance)
	if issuer == nil && authCaIssuer != nil {
		issuer = &registryv1alpha1.CertIssuerRefSpec{
			Name: caIssuerNameForCR(instance),
			Kind: "Issuer",
		}
	}
	return
}

func (r *ReconcileImageRegistry) authCaIssuerRefForCR(instance *registryv1alpha1.ImageRegistry) (issuer *registryv1alpha1.CertIssuerRefSpec) {
	issuer = instance.Spec.Auth.IssuerRef
	if issuer == nil && r.defaultClusterIssuer != "" {
		issuer = &registryv1alpha1.CertIssuerRefSpec{
			Name: r.defaultClusterIssuer,
			Kind: "ClusterIssuer",
		}
	}
	return
}

func toObjectReference(issuer *registryv1alpha1.CertIssuerRefSpec) certmgrmeta.ObjectReference {
	return certmgrmeta.ObjectReference{
		Name: issuer.Name,
		Kind: issuer.Kind,
	}
}
