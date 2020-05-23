package imageregistry

import (
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/status"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	annotationExternalDnsHostname     = "external-dns.alpha.kubernetes.io/hostname"
	annotationImageRegistryGeneration = "registry.mgoltzsche.github.com/generation"
	annotationStatefulSetExternalName = "registry.mgoltzsche.github.com/externalName"
)

func (r *ReconcileImageRegistry) reconcileService(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	svc := &corev1.Service{}
	svc.Name = instance.Name
	svc.Namespace = instance.Namespace
	return r.upsert(instance, svc, reqLogger, func() error {
		externalHostname := r.externalHostnameForCR(instance)
		svc.Annotations[annotationExternalDnsHostname] = externalHostname
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "https",
				Port:       443,
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 443},
				Protocol:   corev1.ProtocolTCP,
			},
		}
		svc.Spec.Selector = selectorLabelsForCR(instance)
		return nil
	})
}

func (r *ReconcileImageRegistry) reconcilePersistentVolumeClaim(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = pvcNameForCR(instance)
	pvc.Namespace = instance.Namespace
	storageClassName := instance.Spec.PersistentVolumeClaim.StorageClassName
	accessModes := instance.Spec.PersistentVolumeClaim.AccessModes
	if len(instance.Spec.PersistentVolumeClaim.AccessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
	}
	/*ctx := context.TODO()
	key := types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}
	if err = r.client.Get(ctx, key, pvc); err != nil {
		if errors.IsNotFound(err) {
			pvc.Spec.StorageClassName = storageClassName
			pvc.Spec.AccessModes = accessModes
			pvc.Spec.Resources = instance.Spec.PersistentVolumeClaim.Resources
			err = r.client.Create(ctx, pvc)
		}
		return
	}

	if (storageClassName != nil && pvc.Spec.StorageClassName != storageClassName) || len(pvc.Spec.AccessModes) == 0 || pvc.Spec.AccessModes[0] != accessModes[0] {
		return fmt.Errorf("%s", "pvc storageClassName or accessMode changed (all pvc fields except resource requests are immutable)")
	}
	patch := client.MergeFrom(pvc.DeepCopy())
	pvc.Spec.Resources = instance.Spec.PersistentVolumeClaim.Resources
	return r.client.Patch(ctx, pvc, patch)*/

	return r.upsert(instance, pvc, reqLogger, func() error {
		if storageClassName != nil {
			pvc.Spec.StorageClassName = storageClassName
		}
		pvc.Spec.AccessModes = accessModes
		pvc.Spec.Resources = instance.Spec.PersistentVolumeClaim.Resources
		return nil
	})
}

func (r *ReconcileImageRegistry) reconcileStatefulSet(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	statefulSet := &appsv1.StatefulSet{}
	statefulSet.Name = instance.Name
	statefulSet.Namespace = instance.Namespace
	return r.upsert(instance, statefulSet, reqLogger, func() error {
		externalName := r.externalHostnameForCR(instance)
		generation := strconv.FormatInt(instance.Generation, 10)
		a := statefulSet.Annotations
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
		condStatus := corev1.ConditionTrue
		var condReason status.ConditionReason
		condMsg := ""
		if !ready {
			condStatus = corev1.ConditionFalse
			condMsg = fmt.Sprintf("%d/%d pods updating", updatedReplicas, replicas)
			condReason = ReasonUpdating
		}
		instance.Status.Conditions.SetCondition(status.Condition{
			Type:    ConditionReady,
			Status:  condStatus,
			Message: condMsg,
			Reason:  condReason,
		})

		a[annotationImageRegistryGeneration] = generation
		a[annotationStatefulSetExternalName] = externalName

		return nil
	})
}

func (r *ReconcileImageRegistry) updateStatefulSetForCR(cr *registryv1alpha1.ImageRegistry, statefulSet *appsv1.StatefulSet) {
	labels := selectorLabelsForCR(cr)
	replicas := int32(1)
	if cr.Spec.Replicas != nil {
		replicas = *cr.Spec.Replicas
	}
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
	if cr.Spec.Auth.ConfigMapName != nil {
		authConfigMapVol := "auth-config"
		volumes = append(volumes, corev1.Volume{
			Name: authConfigMapVol,
			VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: *cr.Spec.Auth.ConfigMapName},
			}},
		})
		authVolumeMounts = append(authVolumeMounts,
			corev1.VolumeMount{Name: authConfigMapVol, MountPath: "/config"})
	}
	statefulSet.Spec = appsv1.StatefulSetSpec{
		Replicas:    &replicas,
		ServiceName: serviceNameForCR(cr),
		UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		},
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: corev1.PodSpec{
				ServiceAccountName: serviceAccountNameForCR(cr),
				RestartPolicy:      corev1.RestartPolicyAlways,
				Volumes:            volumes,
				Containers: []corev1.Container{
					{
						Name:            "registry",
						Image:           r.imageRegistry,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports: []corev1.ContainerPort{
							{Name: "docker", ContainerPort: 5000, Protocol: corev1.ProtocolTCP},
						},
						Env: []corev1.EnvVar{
							{Name: "REGISTRY_HTTP_ADDR", Value: ":5000"},
							{Name: "REGISTRY_HTTP_RELATIVEURLS", Value: "true"},
							{Name: "REGISTRY_STORAGE_DELETE_ENABLED", Value: "true"},
							{Name: "REGISTRY_AUTH", Value: "token"},
							{Name: "REGISTRY_AUTH_TOKEN_REALM", Value: r.externalUrlForCR(cr) + "/auth/token"},
							{Name: "REGISTRY_AUTH_TOKEN_AUTOREDIRECT", Value: "true"},
							{Name: "REGISTRY_AUTH_TOKEN_ISSUER", Value: "Docker Registry Auth Service"},
							{Name: "REGISTRY_AUTH_TOKEN_SERVICE", Value: "Docker Registry"},
							{Name: "REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE", Value: "/root/auth-cert/ca.crt"},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "images", MountPath: "/var/lib/registry"},
							{Name: "registry-auth-token-ca", MountPath: "/root/auth-cert"},
						},
						LivenessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/",
									Port: intstr.IntOrString{Type: intstr.Int, IntVal: 5000},
								},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       3,
						},
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
						Image:           r.imageAuth,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Args: []string{
							"--v=2",
							"--alsologtostderr",
							"/config/auth_config.yml",
						},
						Env: []corev1.EnvVar{
							{Name: "NAMESPACE", Value: cr.GetNamespace()},
						},
						VolumeMounts: authVolumeMounts,
						Ports: []corev1.ContainerPort{
							{Name: "auth", ContainerPort: 5001, Protocol: corev1.ProtocolTCP},
						},
						LivenessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/",
									Port: intstr.IntOrString{Type: intstr.Int, IntVal: 5000},
								},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       3,
						},
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
						Image:           r.imageNginx,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports: []corev1.ContainerPort{
							{Name: "https", ContainerPort: 443, Protocol: corev1.ProtocolTCP},
							{Name: "http", ContainerPort: 80, Protocol: corev1.ProtocolTCP},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "tls", MountPath: "/etc/nginx/tls"},
						},
						LivenessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.IntOrString{Type: intstr.Int, IntVal: 80},
								},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       3,
						},
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
				},
			},
		},
	}
}
