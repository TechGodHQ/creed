package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techgodhq/creed/internal/service"
)

// TestLayeredGitHubAuthIntegration is opt-in because it requires credentials
// owned by the CI/user environment. It exercises the complete layered service
// path rather than only testing the adapter's auth-method type.
func TestLayeredGitHubAuthIntegration(t *testing.T) {
	if os.Getenv("CREED_RUN_GITHUB_AUTH_INTEGRATION") != "1" {
		t.Skip("set CREED_RUN_GITHUB_AUTH_INTEGRATION=1 for GitHub auth integration")
	}

	httpsRemote := strings.TrimSpace(os.Getenv("CREED_GITHUB_HTTPS_REMOTE"))
	httpsToken := os.Getenv("CREED_GITHUB_HTTPS_TOKEN")
	sshRemote := strings.TrimSpace(os.Getenv("CREED_GITHUB_SSH_REMOTE"))
	if httpsRemote == "" && sshRemote == "" {
		t.Fatal("set CREED_GITHUB_HTTPS_REMOTE and/or CREED_GITHUB_SSH_REMOTE")
	}
	if httpsRemote != "" {
		if !strings.HasPrefix(strings.ToLower(httpsRemote), "https://github.com/") {
			t.Fatal("CREED_GITHUB_HTTPS_REMOTE must point to github.com over HTTPS")
		}
		if httpsToken == "" {
			t.Fatal("CREED_GITHUB_HTTPS_TOKEN is required for the HTTPS integration")
		}
		runLayeredGitHubValidation(t, httpsRemote, httpsToken)
	}
	if sshRemote != "" {
		lower := strings.ToLower(sshRemote)
		if !strings.HasPrefix(lower, "git@github.com:") && !strings.HasPrefix(lower, "ssh://git@github.com/") {
			t.Fatal("CREED_GITHUB_SSH_REMOTE must point to github.com over SSH")
		}
		runLayeredGitHubValidation(t, sshRemote, "")
	}
}

func runLayeredGitHubValidation(t *testing.T, remote, token string) {
	t.Helper()
	root := t.TempDir()
	manifest := fmt.Sprintf(`version: 1
source:
  type: layered
  path: .creed
  layers:
    - name: org
      type: git
      remote: %q
      path: .creed
targets: []
`, remote)
	creedDir := filepath.Join(root, ".creed")
	if err := os.MkdirAll(creedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creedDir, "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := service.New(root, service.WithGitToken(token), service.WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Validate(context.Background())
	if err != nil {
		t.Fatalf("layered GitHub validation returned error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("layered GitHub validation failed: %#v", result.Errors)
	}
}
