package torequests

import (
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AnnotationToRequest string

func (c AnnotationToRequest) Map(o handler.MapObject) (r []reconcile.Request) {
	a := o.Meta.GetAnnotations()
	if a == nil {
		return
	}
	fullName := a[string(c)]
	if fullName == "" {
		return
	}
	s := strings.SplitN(fullName, string(types.Separator), 2)
	if len(s) < 2 || s[0] == "" || s[1] == "" {
		return
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: s[0], Name: s[1]}}}
}
