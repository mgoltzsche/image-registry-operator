package imageregistry

import (
	"github.com/go-logr/logr"
	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
)

func (r *ReconcileImageRegistry) reconcileServiceAccount(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	a := &corev1.ServiceAccount{}
	a.Name = serviceAccountNameForCR(instance)
	a.Namespace = instance.Namespace
	return r.upsert(instance, a, reqLogger, func() error { return nil })
}

func (r *ReconcileImageRegistry) reconcileRole(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	role := &rbac.Role{}
	role.Name = serviceAccountNameForCR(instance)
	role.Namespace = instance.Namespace
	return r.upsert(instance, role, reqLogger, func() error {
		role.Rules = []rbac.PolicyRule{
			{
				APIGroups: []string{registryv1alpha1.SchemeGroupVersion.Group},
				Resources: []string{"imageregistryaccounts"},
				Verbs:     []string{"get", "list", "watch"},
			},
		}
		return nil
	})
}

func (r *ReconcileImageRegistry) reconcileRoleBinding(instance *registryv1alpha1.ImageRegistry, reqLogger logr.Logger) (err error) {
	rb := &rbac.RoleBinding{}
	rb.Name = serviceAccountNameForCR(instance)
	rb.Namespace = instance.Namespace
	return r.upsert(instance, rb, reqLogger, func() error {
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
