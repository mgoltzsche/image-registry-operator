package torequests

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMap(t *testing.T) {
	m := NewMap()
	name := types.NamespacedName{Name: "myresource", Namespace: "mynamespace"}
	otherName := types.NamespacedName{Name: "myother", Namespace: "mynamespace"}
	watchedName1 := types.NamespacedName{Name: "watched", Namespace: "watchedns"}
	watchedName2 := types.NamespacedName{Name: "watched2", Namespace: "watchedns"}
	watched1 := &metav1.ObjectMeta{
		Name:      watchedName1.Name,
		Namespace: watchedName1.Namespace,
	}
	watched2 := &metav1.ObjectMeta{
		Name:      watchedName2.Name,
		Namespace: watchedName2.Namespace,
	}

	// Map
	r := m.Map(handler.MapObject{Meta: watched1})
	require.True(t, len(r) == 0, "len(Map()) initially")

	// Put
	m.Put(name, []types.NamespacedName{watchedName1, watchedName2})
	m.Put(otherName, []types.NamespacedName{otherName, watchedName2})
	expectedRequests := []reconcile.Request{{NamespacedName: name}}
	for _, w := range []*metav1.ObjectMeta{watched1, watched2} {
		r = m.Map(handler.MapObject{Meta: w})
		require.Equal(t, expectedRequests, r, "Map()")
		expectedRequests = append(expectedRequests, reconcile.Request{NamespacedName: otherName})
	}

	// Del
	m.Del(name)
	r = m.Map(handler.MapObject{Meta: watched2})
	require.Equal(t, []reconcile.Request{{NamespacedName: otherName}}, r, "Map() after Del()")
}
