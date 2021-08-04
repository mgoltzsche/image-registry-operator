/*
Copyright 2021 Max Goltzsche.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reg8stry

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/mgoltzsche/reg8stry/internal/certs"
)

// CARootCertificateSecretReconciler reconciles a Secret object that holds the generated CA root certificate.
type CARootCertificateSecretReconciler struct {
	client           client.Client
	scheme           *runtime.Scheme
	CARootSecretName types.NamespacedName
	CertManager      *certs.CertManager
}

// SetupWithManager sets up the controller with the Manager.
func (r *CARootCertificateSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	_, err := r.CertManager.RenewRootCACertSecret()
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}, builder.WithPredicates(filterName(r.CARootSecretName))).
		Complete(r)
}

//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *CARootCertificateSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info("Reconciling CA root certificate Secret")

	cert, err := r.CertManager.RenewRootCACertSecret()
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: cert.NextRenewal().Sub(time.Now()) + 10*time.Second}, nil
}

func filterName(name types.NamespacedName) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetName() == name.Name && o.GetNamespace() == name.Namespace
	})
}
