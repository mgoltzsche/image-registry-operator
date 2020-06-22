package imagebuildenv

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type resourceToRequestsMap struct {
	mapping map[string][]reconcile.Request
	mutex   *sync.Mutex
}

func newRequestMap() resourceToRequestsMap {
	return resourceToRequestsMap{map[string][]reconcile.Request{}, &sync.Mutex{}}
}

// Map maps a given object (by namespaced name) to reconcile requests
func (m resourceToRequestsMap) Map(o handler.MapObject) (r []reconcile.Request) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	meta := o.Meta
	if meta == nil {
		return
	}
	key := fmt.Sprintf("%s/%s", meta.GetNamespace(), meta.GetName())
	return m.mapping[key]
}

// Add mapping from secret to instance or rather makes instance watch secret
func (m resourceToRequestsMap) Put(instance types.NamespacedName, secrets []types.NamespacedName) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.del(instance)
	for _, s := range secrets {
		m.put(instance, s)
	}
}

// Del removes instance's secret mappings
func (m resourceToRequestsMap) Del(instance types.NamespacedName) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.del(instance)
}

func (m resourceToRequestsMap) put(instance, secret types.NamespacedName) {
	key := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
	r := m.mapping[key]
	if r == nil {
		m.mapping[key] = []reconcile.Request{{instance}}
		return // new key created
	}
	for _, v := range r {
		if instance.Name == v.Name && instance.Namespace == v.Namespace {
			return // already exists
		}
	}
	// add request
	r = append(r, reconcile.Request{instance})
	m.mapping[key] = r
}

func (m resourceToRequestsMap) del(instance types.NamespacedName) {
	for k, v := range m.mapping {
		filtered := make([]reconcile.Request, 0, len(v))
		for _, r := range v {
			if instance.Name != r.Name || instance.Namespace != r.Namespace {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) < len(v) {
			if len(filtered) == 0 {
				delete(m.mapping, k)
			} else {
				m.mapping[k] = filtered
			}
		}
	}
}
