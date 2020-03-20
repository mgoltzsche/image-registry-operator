package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func WaitForCondition(t *testing.T, obj runtime.Object, name, ns string, pollTimeout time.Duration, condition func() []string) (err error) {
	t.Logf("waiting up to %v for %s to become ready", pollTimeout, name)
	return wait.PollImmediate(time.Second, pollTimeout, func() (bool, error) {
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
}
