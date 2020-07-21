package e2e

import (
	"testing"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/require"
)

func testAuthentication(t *testing.T, ctx *framework.Context, cr, user, pw string, expectedLabels map[string][]string, auth func(t *testing.T, ns, usr, pw string, succeed bool) map[string][]string) {
	ns := framework.Global.Namespace
	t.Run("missing authn", func(t *testing.T) {
		labels := auth(t, ns, "", "", false)
		require.Nil(t, labels, "labels")
	})
	t.Run("invalid authn", func(t *testing.T) {
		labels := auth(t, ns, "unknown", "invalid", false)
		require.Nil(t, labels, "labels")
	})
	t.Run("valid authn", func(t *testing.T) {
		labels := auth(t, ns, user, pw, true)
		require.Equal(t, expectedLabels, labels, "labels")
	})
}
