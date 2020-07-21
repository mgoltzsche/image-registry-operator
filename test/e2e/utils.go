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

func WaitForCondition(t *testing.T, obj runtime.Object, pollTimeout time.Duration, condition func() []string) (err error) {
	key, err := dynclient.ObjectKeyFromObject(obj)
	if err != nil {
		return
	}
	t.Logf("waiting up to %v for %s condition...", pollTimeout, key.Name)
	err = wait.PollImmediate(time.Second, pollTimeout, func() (bool, error) {
		if e := framework.Global.Client.Get(context.TODO(), key, obj); e != nil {
			//t.Logf("%s not found: %s", key, e)
			//return false, nil
			return false, e
		}
		if c := condition(); len(c) > 0 {
			t.Logf("  %s did not meet condition: %v", key, c)
			return false, nil
		}
		t.Logf("%s met condition", key)
		return true, nil
	})
	if err != nil {
		t.Logf("  ERROR: %s. printing events...", err)
		printEvents(t, key.Namespace)
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
		//if evt.Type != "Normal" {
		secondsAgo := now.Sub(evt.CreationTimestamp.Time).Seconds()
		evts = append(evts, fmt.Sprintf("%4.0fs ago: %s/%s: %s: %s: %s", secondsAgo, evt.Regarding.Kind, evt.Regarding.Name, evt.Type, evt.Reason, evt.Note))
		//}
	}
	sort.Reverse(sort.StringSlice(evts))
	for _, evt := range evts {
		t.Logf("    %s", evt)
	}
}
