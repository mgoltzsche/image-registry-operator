package imagebuildenv

import (
	registryv1alpha1 "github.com/mgoltzsche/image-registry-operator/pkg/apis/registry/v1alpha1"
	"github.com/mgoltzsche/image-registry-operator/pkg/backrefs"
	corev1 "k8s.io/api/core/v1"
)

type referenceOwner struct {
	*registryv1alpha1.ImageBuildEnv
}

func (s *referenceOwner) GetStatusReferences() []backrefs.Object {
	refs := make([]backrefs.Object, len(s.Status.SecretRefs))
	for i, name := range s.Status.SecretRefs {
		sec := &corev1.Secret{}
		sec.Name = name
		sec.Namespace = s.Namespace
		refs[i] = sec
	}
	return refs
}

func (s *referenceOwner) SetStatusReferences(refs []backrefs.Object) {
	names := make([]string, len(refs))
	for i, ref := range refs {
		names[i] = ref.GetName()
	}
	s.Status.SecretRefs = names
}

func (owner *referenceOwner) GetObject() backrefs.Object {
	return owner.ImageBuildEnv
}
