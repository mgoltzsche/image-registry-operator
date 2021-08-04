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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	registryapi "github.com/mgoltzsche/reg8stry/apis/reg8stry/v1alpha1"
)

// ImageRegistryAccountReconciler reconciles an ImageRegistryAccount object
type ImageRegistryAccountReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *ImageRegistryAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&registryapi.ImageRegistryAccount{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=reg8stry.mgoltzsche.github.com,resources=imageregistryaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=reg8stry.mgoltzsche.github.com,resources=imageregistryaccounts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=reg8stry.mgoltzsche.github.com,resources=imageregistryaccounts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *ImageRegistryAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info("Reconciling ImageRegistryAccount")

	// Fetch the ImageRegistryAccount instance
	account := &registryapi.ImageRegistryAccount{}
	err := r.client.Get(ctx, req.NamespacedName, account)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Delete account if expired
	if ttl := account.Spec.TTL; ttl != nil {
		if account.Expired() {
			reqLogger.Info("Deleting expired ImageRegistryAccount", "ImageRegistryAccount.Namespace", account.Namespace, "ImageRegistryAccount.Name", account.Name)
			err = r.client.Delete(ctx, account)
			return ctrl.Result{}, err
		}
		expiryTime := account.CreationTimestamp.Time.Add(ttl.Duration)
		return ctrl.Result{RequeueAfter: expiryTime.Sub(time.Now()) + 10*time.Second}, nil
	}

	return ctrl.Result{}, nil
}
