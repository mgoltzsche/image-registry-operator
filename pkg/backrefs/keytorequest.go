package backrefs

import (
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const separator = "."

func ToMapValue(name types.NamespacedName) string {
	return name.Name + separator + name.Namespace
}

func AnnotationToRequest(annotation string) handler.Mapper {
	return &keyToRequest{annotations, annotation}
}

func LabelToRequest(label string) handler.Mapper {
	return &keyToRequest{labels, label}
}

type objToKeys func(o handler.MapObject) map[string]string

type keyToRequest struct {
	keyFn objToKeys
	key   string
}

func (m *keyToRequest) Map(o handler.MapObject) (r []reconcile.Request) {
	a := m.keyFn(o)
	if a == nil {
		return
	}
	fullName := a[m.key]
	if fullName == "" {
		return
	}
	s := strings.SplitN(fullName, separator, 2)
	if len(s) < 2 || s[0] == "" || s[1] == "" {
		return
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: s[1], Name: s[0]}}}
}

func annotations(o handler.MapObject) map[string]string {
	return o.Meta.GetAnnotations()
}

func labels(o handler.MapObject) map[string]string {
	return o.Meta.GetLabels()
}
