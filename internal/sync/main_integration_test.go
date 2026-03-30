//go:build integration

package sync_test

import (
	"testing"

	"breadbox/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}
