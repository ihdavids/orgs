package orgs

import (
	"os"
	"testing"

	"gopkg.in/op/go-logging.v1"
)

// TestMain keeps the server's info chatter (LOAD FILE: for every temp file a
// test writes) out of a failing test's output. ORGS_TEST_LOG=1 brings it back.
func TestMain(m *testing.M) {
	if os.Getenv("ORGS_TEST_LOG") == "" {
		logging.SetLevel(logging.WARNING, "")
	}
	os.Exit(m.Run())
}
