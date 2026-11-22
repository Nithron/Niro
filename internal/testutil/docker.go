// Package testutil holds helpers shared by tests.
package testutil

import (
	"os"
	"os/exec"
	"testing"
)

// RequireDocker skips integration tests when Docker is unavailable, unless
// NITHRO_REQUIRE_INTEGRATION=1 (set in CI) turns the skip into a failure.
func RequireDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		if os.Getenv("NITHRO_REQUIRE_INTEGRATION") == "1" {
			t.Fatalf("docker is required for integration tests: %v", err)
		}
		t.Skipf("docker unavailable, skipping integration test: %v", err)
	}
}
