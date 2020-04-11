package e2e

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func WaitForCondition(t *testing.T, obj runtime.Object, name, ns string, pollTimeout time.Duration, condition func() []string) (err error) {
	t.Logf("waiting up to %v for %s to become ready", pollTimeout, name)
	err = wait.PollImmediate(time.Second, pollTimeout, func() (bool, error) {
		key := dynclient.ObjectKey{Name: name, Namespace: ns}
		if err = framework.Global.Client.Get(context.TODO(), key, obj); err != nil {
			err = fmt.Errorf("%s not found: %s", key, err)
			t.Logf(err.Error())
			return false, nil
		}
		if c := condition(); len(c) > 0 {
			err = fmt.Errorf("  %s did not meet condition: %v", key, c)
			t.Logf(err.Error())
			return false, nil
		}
		t.Logf("%s met condition", key)
		return true, nil
	})
	if err != nil {
		t.Logf("  ERROR: %s. printing events...", err)
		printEvents(t, framework.Global.Namespace)
	}
	return
}

func printEvents(t *testing.T, namespace string) {
	evtList, err := framework.Global.KubeClient.EventsV1beta1().Events(namespace).List(metav1.ListOptions{})
	if err != nil {
		t.Logf("    cannot list events: %s", err)
		return
	}
	evts := []string{}
	now := time.Now()
	for _, evt := range evtList.Items {
		if evt.Type != "Normal" {
			secondsAgo := now.Sub(evt.CreationTimestamp.Time).Seconds()
			evts = append(evts, fmt.Sprintf("%4.0fs ago: %s/%s: %s: %s: %s", secondsAgo, evt.Regarding.Kind, evt.Regarding.Name, evt.Type, evt.Reason, evt.Note))
		}
	}
	sort.Reverse(sort.StringSlice(evts))
	for _, evt := range evts {
		t.Logf("    %s", evt)
	}
}
