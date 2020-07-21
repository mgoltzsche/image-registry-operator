package backrefs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const testAnnotation = "backrefhandler"

func TestBackReferenceHandler(t *testing.T) {
	logger := logf.Log
	ctx := context.TODO()
	client := fake.NewFakeClient()
	owner := &mockOwner{&corev1.ConfigMap{}}
	owner.Namespace = "ns0"
	owner.Name = "myconf"
	refs := []Object{}
	for i := 0; i < 6; i++ {
		if i%2 == 0 {
			ref := &corev1.Secret{}
			ref.Namespace = fmt.Sprintf("ns%d", i%2)
			ref.Name = fmt.Sprintf("secret%d", i%5)
			refs = append(refs, ref)
		}
	}
	allObj := append(refs, owner.ConfigMap)
	for _, o := range allObj {
		err := client.Create(ctx, o)
		require.NoError(t, err, "create test resource")
	}
	loadObjects(t, client, allObj)

	testee := NewBackReferencesHandler(client, OwnerReferences())

	retrievedRefs := owner.GetStatusReferences()
	require.Equal(t, 0, len(retrievedRefs), "initial len(refs)")

	for _, c := range []struct {
		refs []Object
		name string
	}{
		{refs, "add first refs"},
		{refs[2:], "remove refs"},
		{refs, "add all refs again"},
		{nil, "remove all refs"},
	} {
		t.Log(c.name)
		err := testee.UpdateReferences(ctx, logger, owner, c.refs)
		require.NoError(t, err, "UpdateReferences")
		loadObjects(t, client, allObj)
		versions := toVersionMap(allObj)

		retrievedRefs = owner.GetStatusReferences()
		loadObjects(t, client, retrievedRefs)
		require.Equal(t, keys(c.refs), keys(retrievedRefs), "owner status refs")

		backRefSecrets := secretsByOwnerRef(c.refs, owner.GetObject())
		require.Equal(t, keys(c.refs), backRefSecrets, "back references (secrets->configmap)")

		err = testee.UpdateReferences(ctx, logger, owner, c.refs)
		require.NoError(t, err, "UpdateReferences without changes")
		loadObjects(t, client, allObj)

		require.Equal(t, versions, toVersionMap(allObj), "resource versions changed after update without changes")
	}
}

func loadObjects(t *testing.T, c runtimeclient.Client, l []Object) {
	for _, o := range l {
		key := types.NamespacedName{Name: o.GetName(), Namespace: o.GetNamespace()}
		err := c.Get(context.TODO(), key, o)
		require.NoError(t, err, "fetch test resource")
	}
}

func toVersionMap(l []Object) map[string]string {
	m := map[string]string{}
	for _, o := range l {
		m[key(o)] = o.GetResourceVersion()
	}
	return m
}

type mockOwner struct {
	*corev1.ConfigMap
}

func (o *mockOwner) GetStatusReferences() (r []Object) {
	a := o.Annotations
	if a != nil {
		for k, v := range o.Annotations {
			if strings.HasPrefix(k, testAnnotation) && v == "true" {
				l := strings.SplitN(k, "/", 4)
				if len(l) == 4 {
					sec := &corev1.Secret{}
					sec.Namespace = l[2]
					sec.Name = l[3]
					r = append(r, sec)
				}
			}
		}
	}
	return
}

func (o *mockOwner) SetStatusReferences(refs []Object) {
	a := map[string]string{}
	for _, key := range keys(refs) {
		a[key] = "true"
	}
	o.Annotations = a
}

func (o *mockOwner) GetObject() Object {
	return o.ConfigMap
}

func keys(refs []Object) (r []string) {
	for _, ref := range refs {
		r = append(r, key(ref))
	}
	sort.Strings(r)
	return
}

func key(o Object) string {
	kind := o.GetObjectKind().GroupVersionKind().Kind
	checkKind(kind)
	ns := o.GetNamespace()
	name := o.GetName()
	return fmt.Sprintf("%s/%s/%s/%s", testAnnotation, kind, ns, name)
}

func secretsByOwnerRef(secrets []Object, o Object) (r []string) {
	for _, s := range secrets {
		for i := 0; i < ownerRefCount(s, o); i++ {
			r = append(r, key(s))
		}
	}
	sort.Strings(r)
	return
}

func ownerRefCount(s, o Object) (c int) {
	checkKind(s.GetObjectKind().GroupVersionKind().Kind)
	checkKind(o.GetObjectKind().GroupVersionKind().Kind)
	for _, ref := range s.GetOwnerReferences() {
		if ref.UID == o.GetUID() && ref.Name == o.GetName() &&
			ref.Kind == o.GetObjectKind().GroupVersionKind().Kind {
			c++
		}
	}
	return
}
