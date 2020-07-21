package backrefs

// Copied from https://github.com/mgoltzsche/ktransform

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Object interface {
	runtime.Object
	metav1.Object
}

type Owner interface {
	GetStatusReferences() []Object
	SetStatusReferences(refs []Object)
	GetObject() Object
}

/*type ResourceRef interface {
	GetApiGroup() string
	GetKind() string
	GetName() string
	GetNamespace() string
}*/

type BackReferenceStrategy interface {
	AddReference(from metav1.Object, to Object) bool
	DelReference(from metav1.Object, to Object) bool
}

type BackReferencesHandler struct {
	client   client.Client
	backRefs BackReferenceStrategy
}

func NewBackReferencesHandler(client client.Client, backrefs BackReferenceStrategy) *BackReferencesHandler {
	return &BackReferencesHandler{client, backrefs}
}

// UpdateReferences updates back references from other objects to the owner consistently
func (h *BackReferencesHandler) UpdateReferences(ctx context.Context, logger logr.InfoLogger, owner Owner, refs []Object) (err error) {
	lastRefs := owner.GetStatusReferences()
	delRefs, addRefs := diffRefs(lastRefs, refs)
	// Delete backreferences on previously referenced resources
	err = h.deleteOldRefs(ctx, logger, owner.GetObject(), delRefs)
	if err != nil {
		return
	}
	// Add new refs to owner status
	// (for consistency this needs to happen before setting back references)
	if len(delRefs) > 0 || len(addRefs) > 0 {
		owner.SetStatusReferences(refs)
		err = h.client.Status().Update(ctx, owner.GetObject())
		if err != nil {
			return
		}
	}
	// Set backreferences on referenced resources (to watch them)
	err = h.addNewRefs(ctx, logger, owner.GetObject(), refs)
	return
}

func (h *BackReferencesHandler) addNewRefs(ctx context.Context, logger logr.InfoLogger, owner Object, newRefs []Object) error {
	for _, ref := range newRefs {
		if h.backRefs.AddReference(ref, owner) {
			kind := reflect.TypeOf(ref).Elem().Name()
			logger.Info("Adding back reference to "+kind, kind+".Name", ref.GetName(), kind+".Namespace", ref.GetNamespace())
			if err := h.client.Update(ctx, ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *BackReferencesHandler) deleteOldRefs(ctx context.Context, logger logr.InfoLogger, owner Object, oldRefs []Object) error {
	for _, ref := range oldRefs {
		key := types.NamespacedName{Name: ref.GetName(), Namespace: ref.GetNamespace()}
		err := h.client.Get(ctx, key, ref)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			} else {
				return err
			}
		}
		if h.backRefs.DelReference(ref, owner) {
			kind := reflect.TypeOf(ref).Elem().Name()
			logger.Info("Removing back reference from "+kind, kind+".Name", ref.GetName(), kind+".Namespace", ref.GetNamespace())
			if err = h.client.Update(ctx, ref); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func diffRefs(oldRefs, newRefs []Object) (del []Object, add []Object) {
	oldRefKeys := refKeys(oldRefs)
	newRefKeys := refKeys(newRefs)
	for _, r := range oldRefs {
		if _, ok := newRefKeys[refKey(r)]; !ok {
			del = append(del, r)
		}
	}
	for _, r := range newRefs {
		if _, ok := oldRefKeys[refKey(r)]; !ok {
			add = append(add, r)
		}
	}
	return
}

func refKeys(refs []Object) map[string]struct{} {
	m := map[string]struct{}{}
	for _, ref := range refs {
		m[refKey(ref)] = struct{}{}
	}
	return m
}

func refKey(ref Object) string {
	return fmt.Sprintf("%T:%s/%s", ref, ref.GetNamespace(), ref.GetName())
}
