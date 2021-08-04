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
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	certmgr "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha3"
	certmgrmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	registryv1alpha1 "github.com/mgoltzsche/reg8stry/apis/reg8stry/v1alpha1"
	"github.com/mgoltzsche/reg8stry/internal/backrefs"
	"github.com/mgoltzsche/reg8stry/internal/certs"
	"github.com/mgoltzsche/reg8stry/internal/status"
)

const (
	annotationImageRegistry           = "registry.mgoltzsche.github.com/imageregistry"
	annotationImageRegistryGeneration = "registry.mgoltzsche.github.com/generation"
	annotationStatefulSetExternalName = "registry.mgoltzsche.github.com/externalName"
	annotationExternalDnsHostname     = "external-dns.alpha.kubernetes.io/hostname"
	internalPortRegistry              = int32(5000)
	internalPortAuth                  = int32(5001)
	internalPortNginx                 = int32(8443)
	publicPortNginx                   = int32(443)
	publicPortName                    = "https"
)

// ImageRegistryReconciler reconciles an ImageRegistry object
type ImageRegistryReconciler struct {
	// Injected from manager
	client.Client
	Scheme *runtime.Scheme
	// Must be provided
	CertManager    *certs.CertManager
	DNSZone        string
	ImageAuth      string
	ImageNginx     string
	ImageRegistry  string
	reconcileTasks []reconcileTask
}

type reconcileTask func(context.Context, *registryv1alpha1.ImageRegistry, logr.Logger) error

// SetupWithManager sets up the controller with the Manager.
func (r *ImageRegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	r.Scheme = mgr.GetScheme()
	r.reconcileTasks = []reconcileTask{
		r.reconcileTokenCert,
		r.reconcileTLSCert,
		r.reconcileServiceAccount,
		r.reconcileRole,
		r.reconcileRoleBinding,
		r.reconcileService,
		r.reconcilePersistentVolumeClaim,
	}
	registryResource := &registryv1alpha1.ImageRegistry{}
	return ctrl.NewControllerManagedBy(mgr).
		For(registryResource).
		Watches(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    registryResource,
		}).
		Watches(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    registryResource,
		}).
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    registryResource,
		}).
		Watches(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, handler.EnqueueRequestsFromMapFunc(
			backrefs.AnnotationToRequestMapper(annotationImageRegistry),
		)).
		Watches(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    registryResource,
		}).
		Watches(&source.Kind{Type: &rbac.Role{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    registryResource,
		}).
		Watches(&source.Kind{Type: &rbac.RoleBinding{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    registryResource,
		}).
		Complete(r)
}

//+kubebuilder:rbac:groups=registry.mgoltzsche.github.com,resources=imageregistries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=registry.mgoltzsche.github.com,resources=imageregistries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=registry.mgoltzsche.github.com,resources=imageregistries/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=create;get;list;update;patch;delete;watch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=create;get;list;update;patch;delete;watch
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims/status,verbs=get
//+kubebuilder:rbac:groups=core,resources=services,verbs=create;get;list;update;patch;delete;watch
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=create;get;list;update;patch;delete;watch
//+kubebuilder:rbac:groups=core,resources=serviceaccounts/status,verbs=get
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=create;get;list;update;patch;delete;watch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles/status,verbs=get
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=create;get;list;update;patch;delete;watch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *ImageRegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.V(1).Info("Reconciling ImageRegistry")

	// Fetch the ImageRegistry instance
	instance := &registryv1alpha1.ImageRegistry{}
	err = r.Client.Get(ctx, req.NamespacedName, instance)
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

	// Prepare conditions
	conditions := status.NewConditions(instance.Generation, &instance.Status.Conditions, reqLogger)
	condSynced := conditions.Condition(registryv1alpha1.ConditionSynced)
	condReady := conditions.Condition(registryv1alpha1.ConditionReady)

	defer func() {
		// Apply status changes when a condition changes
		if err == nil && conditions.Apply() {
			err = r.Client.Status().Update(ctx, instance)
		}
	}()

	// Run reconcile tasks
	for _, task := range r.reconcileTasks {
		if err = task(ctx, instance, reqLogger); err != nil {
			if !apierrors.IsConflict(err) {
				condSynced.False(registryv1alpha1.ReasonFailedSync, err.Error())
			}
			return ctrl.Result{}, err
		}
	}

	done, err := r.reconcileStatefulSet(ctx, instance, condReady, reqLogger)
	if err != nil {
		if !apierrors.IsConflict(err) {
			condSynced.False(registryv1alpha1.ReasonFailedSync, err.Error())
		}
		return ctrl.Result{}, err
	}
	condSynced.True(registryv1alpha1.ReasonSuccess, "all resources have been applied")
	if !done {
		return ctrl.Result{}, nil
	}

	// Update ImageRegistry status fields
	hostname := r.externalHostnameForCR(instance)
	tlsSecretName := tlsSecretNameForCR(instance)
	changedGeneration := instance.Status.ObservedGeneration != instance.Generation
	changedHost := instance.Status.Hostname != hostname
	changedTLSSecretName := instance.Status.TLSSecretName != tlsSecretName
	changedCond := conditions.Apply()
	if changedCond || changedGeneration || changedHost || changedTLSSecretName {
		instance.Status.ObservedGeneration = instance.Generation
		instance.Status.Hostname = hostname
		instance.Status.TLSSecretName = tlsSecretName
		if e := r.Client.Status().Update(ctx, instance); e != nil {
			return ctrl.Result{}, e
		}
	}

	return ctrl.Result{}, err
}

func (r *ImageRegistryReconciler) externalHostnameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return registryHostname(cr, r.DNSZone)
}

func registryHostname(cr *registryv1alpha1.ImageRegistry, dnsZone string) string {
	return fmt.Sprintf("%s.%s.%s", serviceNameForCR(cr), cr.Namespace, dnsZone)
}

func (r *ImageRegistryReconciler) reconcileTokenCert(ctx context.Context, instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	secretName := authCASecretNameForCR(instance)
	commonName := fmt.Sprintf("%s.%s.svc", instance.Name, instance.Namespace)
	labels := selectorLabelsForCR(instance)
	authTokenCaIssuer := instance.Spec.Auth.CA.IssuerRef
	if authTokenCaIssuer != nil {
		caCertCR := &certmgr.Certificate{}
		caCertCR.Name = authCACertNameForCR(instance)
		caCertCR.Namespace = instance.Namespace
		err = r.upsert(ctx, instance, caCertCR, reqLogger, func() error {
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
		_, err = r.CertManager.RenewCACertSecret(key, instance, labels, commonName)
	}
	return
}

func (r *ImageRegistryReconciler) reconcileTLSCert(ctx context.Context, instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	secretName := tlsSecretNameForCR(instance)
	dnsNames := r.dnsNamesForCR(instance)
	labels := selectorLabelsForCR(instance)
	tlsIssuer := instance.Spec.TLS.IssuerRef
	if tlsIssuer != nil {
		tlsCertCR := &certmgr.Certificate{}
		tlsCertCR.Name = tlsCertNameForCR(instance)
		tlsCertCR.Namespace = instance.Namespace
		err = r.upsert(ctx, instance, tlsCertCR, reqLogger, func() error {
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
		ca, e := r.CertManager.RootCACert()
		if e != nil {
			return e
		}
		key := types.NamespacedName{Name: secretName, Namespace: instance.Namespace}
		_, err = r.CertManager.RenewServerCertSecret(key, instance, labels, dnsNames, ca)
	}
	return
}

func (r *ImageRegistryReconciler) dnsNamesForCR(instance *registryv1alpha1.ImageRegistry) []string {
	dnsNames := []string{}
	internalFQN := fmt.Sprintf("%s.%s.svc.cluster.local", instance.Name, instance.Namespace)
	externalFQN := fmt.Sprintf("%s.%s.%s", instance.Name, instance.Namespace, r.DNSZone)
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

func (r *ImageRegistryReconciler) reconcileServiceAccount(ctx context.Context, instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	a := &corev1.ServiceAccount{}
	a.Name = serviceAccountNameForCR(instance)
	a.Namespace = instance.Namespace
	return r.upsert(ctx, instance, a, reqLogger, func() error { return nil })
}

func (r *ImageRegistryReconciler) reconcileRole(ctx context.Context, instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	role := &rbac.Role{}
	role.Name = serviceAccountNameForCR(instance)
	role.Namespace = instance.Namespace
	return r.upsert(ctx, instance, role, reqLogger, func() error {
		role.Rules = []rbac.PolicyRule{
			{
				APIGroups: []string{registryv1alpha1.GroupVersion.Group},
				Resources: []string{"imageregistryaccounts"},
				Verbs:     []string{"get", "list", "watch"},
			},
		}
		return nil
	})
}

func (r *ImageRegistryReconciler) reconcileRoleBinding(ctx context.Context, instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	rb := &rbac.RoleBinding{}
	rb.Name = serviceAccountNameForCR(instance)
	rb.Namespace = instance.Namespace
	return r.upsert(ctx, instance, rb, reqLogger, func() error {
		rb.Subjects = []rbac.Subject{
			{
				APIGroup: corev1.SchemeGroupVersion.Group,
				Kind:     "ServiceAccount",
				Name:     rb.Name,
			},
		}
		rb.RoleRef.APIGroup = rbac.SchemeGroupVersion.Group
		rb.RoleRef.Kind = "Role"
		rb.RoleRef.Name = rb.Name
		return nil
	})
}

func (r *ImageRegistryReconciler) upsert(ctx context.Context, owner *registryv1alpha1.ImageRegistry, obj client.Object, reqLogger logr.Logger, modify func() error) (err error) {
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if obj.GetUID() == "" && owner != nil {
			if err := ctrl.SetControllerReference(owner, obj, r.Scheme); err != nil {
				return err
			}
		}
		if obj.GetAnnotations() == nil {
			obj.SetAnnotations(map[string]string{})
		}
		return modify()
	})
	if err != nil {
		return err
	}
	switch result {
	case controllerutil.OperationResultCreated:
		logOperation(reqLogger, "Created", obj)
	case controllerutil.OperationResultUpdated:
		logOperation(reqLogger, "Updated", obj)
	}
	return nil
}

func logOperation(log logr.Logger, verb string, o metav1.Object) {
	kind := reflect.TypeOf(o).Elem().Name()
	msg := fmt.Sprintf("%s %s", verb, kind)
	log.Info(msg, kind, fmt.Sprintf("%s/%s", o.GetNamespace(), o.GetName()))
}

func (r *ImageRegistryReconciler) reconcileService(ctx context.Context, instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	svc := &corev1.Service{}
	svc.Name = instance.Name
	svc.Namespace = instance.Namespace
	return r.upsert(ctx, instance, svc, reqLogger, func() error {
		externalHostname := r.externalHostnameForCR(instance)
		svc.Annotations[annotationExternalDnsHostname] = externalHostname
		svc.Spec.Selector = selectorLabelsForCR(instance)
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		addServicePort(svc, publicPortName, publicPortNginx, internalPortNginx, corev1.ProtocolTCP)
		return nil
	})
}

func (r *ImageRegistryReconciler) reconcilePersistentVolumeClaim(ctx context.Context, instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = pvcNameForCR(instance)
	pvc.Namespace = instance.Namespace
	storageClassName := instance.Spec.PersistentVolumeClaim.StorageClassName
	accessModes := instance.Spec.PersistentVolumeClaim.AccessModes
	if len(instance.Spec.PersistentVolumeClaim.AccessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
	}

	var owner *registryv1alpha1.ImageRegistry
	if instance.Spec.PersistentVolumeClaim.DeleteClaim {
		owner = instance
	}

	return r.upsert(ctx, owner, pvc, reqLogger, func() error {
		if owner == nil {
			pvc.SetLabels(selectorLabelsForCR(instance))
			pvc.OwnerReferences = nil
		}
		if storageClassName != nil {
			pvc.Spec.StorageClassName = storageClassName
		}

		pvc.Spec.AccessModes = accessModes
		pvc.Spec.Resources = instance.Spec.PersistentVolumeClaim.Resources

		a := pvc.GetAnnotations()
		if a == nil {
			a = map[string]string{}
		}
		a[string(annotationImageRegistry)] = backrefs.ToMapValue(types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()})
		pvc.SetAnnotations(a)
		return nil
	})
}

func (r *ImageRegistryReconciler) reconcileStatefulSet(ctx context.Context, instance *registryv1alpha1.ImageRegistry, readyCond status.Condition, reqLogger logr.Logger) (done bool, err error) {
	statefulSet := &appsv1.StatefulSet{}
	statefulSet.Name = instance.Name
	statefulSet.Namespace = instance.Namespace
	err = r.upsert(ctx, instance, statefulSet, reqLogger, func() error {
		externalName := r.externalHostnameForCR(instance)
		generation := strconv.FormatInt(instance.Generation, 10)
		a := statefulSet.Annotations
		a[annotationImageRegistryGeneration] = generation
		a[annotationStatefulSetExternalName] = externalName
		r.updateStatefulSetForCR(instance, statefulSet)

		// Set ImageRegistry ready condition
		s := statefulSet.Status
		replicas := int32(1)
		if instance.Spec.Replicas != nil {
			replicas = *instance.Spec.Replicas
		}
		updatedReplicas := s.UpdatedReplicas
		generationUpToDate := s.ObservedGeneration == statefulSet.Generation
		if !generationUpToDate {
			updatedReplicas = 0
		}
		generationUpToDate = generationUpToDate && a[annotationImageRegistryGeneration] == generation
		ready := generationUpToDate &&
			statefulSet.Spec.Replicas != nil &&
			*statefulSet.Spec.Replicas == replicas &&
			s.Replicas == replicas &&
			s.ReadyReplicas == replicas &&
			updatedReplicas == replicas

		if !ready {
			readyCond.False(registryv1alpha1.ReasonUpdating, fmt.Sprintf("%d/%d pods updating", updatedReplicas, replicas))
			return nil
		}
		readyCond.True(registryv1alpha1.ReasonSuccess, "registry is ready")
		done = true
		return nil
	})
	return done, err
}

func (r *ImageRegistryReconciler) updateStatefulSetForCR(cr *registryv1alpha1.ImageRegistry, statefulSet *appsv1.StatefulSet) {
	extHostname := r.externalHostnameForCR(cr)
	externalURL := "https://" + extHostname
	authIssuerName := fmt.Sprintf("Docker Registry Auth %s", extHostname)
	labels := selectorLabelsForCR(cr)
	replicas := int32(1)
	if cr.Spec.Replicas != nil {
		replicas = *cr.Spec.Replicas
	}
	spec := &statefulSet.Spec
	spec.Replicas = &replicas
	spec.ServiceName = serviceNameForCR(cr)
	spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
	}
	spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}
	spec.Template.Labels = labels
	podSpec := &statefulSet.Spec.Template.Spec
	podSpec.ServiceAccountName = serviceAccountNameForCR(cr)
	podSpec.RestartPolicy = corev1.RestartPolicyAlways
	volumes := []corev1.Volume{
		{
			Name: "images",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcNameForCR(cr),
					ReadOnly:  false,
				},
			},
		},
		{
			Name:         "tls",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: tlsSecretNameForCR(cr)}},
		},
		{
			Name:         "registry-auth-token-ca",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: authCASecretNameForCR(cr)}},
		},
	}
	authVolumeMounts := []corev1.VolumeMount{
		{Name: "registry-auth-token-ca", MountPath: "/config/auth-cert"},
	}
	authConfigMapVol := "auth-config"
	if cr.Spec.Auth.ConfigMapName != nil {
		volumes = append(volumes, corev1.Volume{
			Name: authConfigMapVol,
			VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: *cr.Spec.Auth.ConfigMapName},
			}},
		})
		authVolumeMounts = append(authVolumeMounts,
			corev1.VolumeMount{Name: authConfigMapVol, MountPath: "/config"})
	}
	podSpec.Volumes = volumes
	podSpec.Containers = []corev1.Container{
		{
			Name:            "registry",
			Image:           r.ImageRegistry,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Ports: []corev1.ContainerPort{
				{Name: "docker", ContainerPort: internalPortRegistry, Protocol: corev1.ProtocolTCP},
			},
			Env: []corev1.EnvVar{
				{Name: "REGISTRY_HTTP_ADDR", Value: fmt.Sprintf(":%d", internalPortRegistry)},
				{Name: "REGISTRY_HTTP_HOST", Value: externalURL},
				{Name: "REGISTRY_HTTP_RELATIVEURLS", Value: "true"},
				{Name: "REGISTRY_STORAGE_DELETE_ENABLED", Value: "true"},
				{Name: "REGISTRY_AUTH", Value: "token"},
				{Name: "REGISTRY_AUTH_TOKEN_REALM", Value: externalURL + "/auth/token"},
				{Name: "REGISTRY_AUTH_TOKEN_AUTOREDIRECT", Value: "true"},
				{Name: "REGISTRY_AUTH_TOKEN_ISSUER", Value: authIssuerName},
				{Name: "REGISTRY_AUTH_TOKEN_SERVICE", Value: fmt.Sprintf("Docker Registry %s", extHostname)},
				{Name: "REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE", Value: "/root/auth-cert/ca.crt"},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "images", MountPath: "/var/lib/registry"},
				{Name: "registry-auth-token-ca", MountPath: "/root/auth-cert"},
			},
			ReadinessProbe: httpProbe(internalPortRegistry, "/"),
			LivenessProbe:  httpProbe(internalPortRegistry, "/"),
			Resources: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
		},
		{
			Name:            "auth",
			Image:           r.ImageAuth,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Env: []corev1.EnvVar{
				{Name: "NAMESPACE", Value: cr.GetNamespace()},
				{Name: "AUTH_SERVER_ADDR", Value: fmt.Sprintf(":%d", internalPortAuth)},
				{Name: "AUTH_TOKEN_ISSUER", Value: authIssuerName},
			},
			VolumeMounts: authVolumeMounts,
			Ports: []corev1.ContainerPort{
				{Name: "auth", ContainerPort: internalPortAuth, Protocol: corev1.ProtocolTCP},
			},
			ReadinessProbe: httpProbe(internalPortAuth, "/"),
			LivenessProbe:  httpProbe(internalPortAuth, "/"),
			Resources: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
		},
		{
			Name:            "nginx",
			Image:           r.ImageNginx,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Ports: []corev1.ContainerPort{
				{Name: "https", ContainerPort: internalPortNginx, Protocol: corev1.ProtocolTCP},
				{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "tls", MountPath: "/etc/nginx/tls"},
			},
			ReadinessProbe: httpProbe(8080, "/health"),
			LivenessProbe:  httpProbe(8080, "/health"),
			Resources: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
		},
	}
}

func httpProbe(port int32, path string) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.IntOrString{Type: intstr.Int, IntVal: port},
			},
		},
		InitialDelaySeconds: 3,
		PeriodSeconds:       3,
	}
}

func addServicePort(svc *corev1.Service, name string, port, targetPort int32, prot corev1.Protocol) {
	for _, p := range svc.Spec.Ports {
		if p.Name == name && p.Port == port && p.TargetPort.IntVal == targetPort && p.Protocol == prot {
			return // port already exists
		}
	}
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       name,
			Port:       port,
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: targetPort},
			Protocol:   prot,
		},
	}
}

func selectorLabelsForCR(cr *registryv1alpha1.ImageRegistry) map[string]string {
	return map[string]string{"app": "imageregistry-" + cr.Name}
}

func serviceNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return cr.Name
}

func pvcNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return "imageregistry-" + cr.Name + "-pvc"
}

func tlsCertNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return "imageregistry-" + cr.Name + "-tls"
}

func tlsSecretNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	name := cr.Spec.TLS.SecretName
	if name != nil {
		return *name
	}
	if cr.Spec.TLS.IssuerRef == nil {
		return "imageregistry-" + cr.Name + "-selfsigned-tls"
	}
	return "imageregistry-" + cr.Name + "-cm-tls"
}

func authCACertNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return "imageregistry-" + cr.Name + "-auth-ca"
}

func authCASecretNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	name := cr.Spec.Auth.CA.SecretName
	if name != nil {
		return *name
	}
	if cr.Spec.Auth.CA.IssuerRef == nil {
		return "imageregistry-" + cr.Name + "-selfsigned-auth-ca"
	}
	return "imageregistry-" + cr.Name + "-cm-auth-ca"
}

func serviceAccountNameForCR(cr *registryv1alpha1.ImageRegistry) string {
	return "imageregistry-" + cr.Name
}
