package backrefs

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func OwnerReferences() BackReferenceStrategy {
	return &ownerRefs{}
}

type ownerRefs struct{}

func (s *ownerRefs) AddReference(from metav1.Object, to Object) bool {
	checkNamespace(from, to)
	for _, ref := range from.GetOwnerReferences() {
		if ref.UID == to.GetUID() {
			return false
		}
	}
	s.DelReference(from, to)
	refs := from.GetOwnerReferences()
	apiVersion, kind := to.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
	checkKind(kind)
	disabled := false
	refs = append(refs, metav1.OwnerReference{
		APIVersion:         apiVersion,
		Kind:               kind,
		Name:               to.GetName(),
		UID:                to.GetUID(),
		Controller:         &disabled,
		BlockOwnerDeletion: &disabled,
	})
	from.SetOwnerReferences(refs)
	return true
}

func (s *ownerRefs) DelReference(from metav1.Object, to Object) bool {
	checkNamespace(from, to)
	refs := from.GetOwnerReferences()
	for i, ref := range refs {
		if equal(ref, to) {
			from.SetOwnerReferences(append(refs[0:i], refs[i+1:]...))
			return true
		}
	}
	return false
}

func equal(ref metav1.OwnerReference, o Object) bool {
	apiVersion, kind := o.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
	checkKind(kind)
	return ref.Name == o.GetName() && ref.Kind == kind && ref.APIVersion == apiVersion
}

func checkNamespace(a, b metav1.Object) {
	if a.GetNamespace() != b.GetNamespace() {
		panic("owner references across namespaces are not supported")
	}
}

func checkKind(kind string) {
	if kind == "" {
		panic("ref.kind is empty - did you fetch it before passing it to Add/DelReference?")
	}
}
