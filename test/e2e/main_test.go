package e2e

import (
	"testing"

	f "github.com/operator-framework/operator-sdk/pkg/test"
)

// See https://github.com/operator-framework/operator-sdk/blob/master/doc/test-framework/writing-e2e-tests.md

func TestMain(m *testing.M) {
	f.MainEntry(m)
}
