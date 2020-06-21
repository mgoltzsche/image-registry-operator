package imagebuildenv

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type resourceToRequestsMapper struct {
	activeMap *map[string][]reconcile.Request
	interMap  map[string][]reconcile.Request
}

func newRequestMapper() resourceToRequestsMapper {
	active := map[string][]reconcile.Request{}
	return resourceToRequestsMapper{&active, map[string][]reconcile.Request{}}
}

// Map maps a given object (by namespaced name) to reconcile requests
func (m resourceToRequestsMapper) Map(o handler.MapObject) (r []reconcile.Request) {
	meta := o.Meta
	if meta == nil {
		return
	}
	key := fmt.Sprintf("%s/%s", meta.GetNamespace(), meta.GetName())
	return (*m.activeMap)[key]
}

// Apply makes the last changes available to the Map function
func (m resourceToRequestsMapper) Apply() {
	c := map[string][]reconcile.Request{}
	for k, v := range m.interMap {
		c[k] = v
	}
	m.activeMap = &c
}

// Add mapping from secret to instance or rather makes instance watch secret
func (m resourceToRequestsMapper) Add(instance, secret types.NamespacedName) {
	key := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
	r := m.interMap[key]
	if r == nil {
		m.interMap[key] = []reconcile.Request{{instance}}
		return // new key created
	}
	for _, v := range r {
		if instance.Name == v.Name && instance.Namespace == v.Namespace {
			return // already exists
		}
	}
	// add request
	r = append(r, reconcile.Request{instance})
	m.interMap[key] = r
}

// Del removes instance's secret mappings
func (m resourceToRequestsMapper) Del(instance types.NamespacedName) {
	for k, v := range m.interMap {
		filtered := make([]reconcile.Request, 0, len(v))
		for _, r := range v {
			if instance.Name != r.Name || instance.Namespace != r.Namespace {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) < len(v) {
			if len(filtered) == 0 {
				delete(m.interMap, k)
			} else {
				m.interMap[k] = filtered
			}
		}
	}
}
