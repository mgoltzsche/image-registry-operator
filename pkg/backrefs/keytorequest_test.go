package backrefs

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestToMapValue(t *testing.T) {
	require.Equal(t, "myname.myns", ToMapValue(types.NamespacedName{Name: "myname", Namespace: "myns"}))
}

func TestAnnotationToRequest(t *testing.T) {
	testKeyToRequest(t, AnnotationToRequest, func(o metav1.Object, input map[string]string) {
		o.SetAnnotations(input)
	})
}

func TestLabelToRequest(t *testing.T) {
	testKeyToRequest(t, LabelToRequest, func(o metav1.Object, input map[string]string) {
		o.SetLabels(input)
	})
}

func testKeyToRequest(t *testing.T, testeeFn func(string) handler.Mapper, setTestData func(metav1.Object, map[string]string)) {
	key := "mykey"
	testee := testeeFn(key)
	refName := types.NamespacedName{Name: "myresource", Namespace: "mynamespace"}
	for _, c := range []struct {
		input    map[string]string
		expected []reconcile.Request
	}{
		{map[string]string{key: refName.Name + separator + refName.Namespace}, []reconcile.Request{{NamespacedName: refName}}},
		{map[string]string{key: "ns" + separator}, nil},
		{map[string]string{key: "ns"}, nil},
		{map[string]string{key: ""}, nil},
		{map[string]string{}, nil},
		{nil, nil},
	} {
		o := handler.MapObject{Meta: &metav1.ObjectMeta{}}
		setTestData(o.Meta, c.input)
		r := testee.Map(o)
		require.Equal(t, c.expected, r, "mapped requests")
	}
}
