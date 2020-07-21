package merge

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func HasFinalizer(o metav1.Object, finalizer string) bool {
	for _, f := range o.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}
