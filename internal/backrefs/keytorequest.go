package backrefs

import (
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const separator = "."

func ToMapValue(name types.NamespacedName) string {
	return name.Name + separator + name.Namespace
}

func AnnotationToRequestMapper(annotation string) handler.MapFunc {
	return mapFunc(annotations, annotation)
}

func LabelToRequestMapper(label string) handler.MapFunc {
	return mapFunc(labels, label)
}

type objToKeys func(o client.Object) map[string]string

func mapFunc(keysFn objToKeys, key string) handler.MapFunc {
	return func(o client.Object) (r []reconcile.Request) {
		a := keysFn(o)
		if a == nil {
			return
		}
		fullName := a[key]
		if fullName == "" {
			return
		}
		s := strings.SplitN(fullName, separator, 2)
		if len(s) < 2 || s[0] == "" || s[1] == "" {
			return
		}
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: s[1], Name: s[0]}}}
	}
}

func annotations(o client.Object) map[string]string {
	return o.GetAnnotations()
}

func labels(o client.Object) map[string]string {
	return o.GetLabels()
}
