package backrefs

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestAnnotationToRequest(t *testing.T) {
	a := "myannotation"
	m := AnnotationToRequest(a)
	refName := types.NamespacedName{Name: "myresource", Namespace: "mynamespace"}
	for _, c := range []struct {
		annotations map[string]string
		expected    []reconcile.Request
	}{
		{map[string]string{a: refName.String()}, []reconcile.Request{{NamespacedName: refName}}},
		{map[string]string{a: "ns" + string(types.Separator)}, nil},
		{map[string]string{a: "ns"}, nil},
		{map[string]string{a: ""}, nil},
		{map[string]string{}, nil},
		{nil, nil},
	} {
		o := handler.MapObject{Meta: &metav1.ObjectMeta{Annotations: c.annotations}}
		r := m.Map(o)
		require.Equal(t, c.expected, r, "mapped requests")
	}
}
