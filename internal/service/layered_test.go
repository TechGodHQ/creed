package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techgodhq/creed/internal/usecase"
)

func TestLayeredSyncConcatenatesOrgBeforeRepoContext(t *testing.T) {
	remote := newLayeredRemote(t, `version: 1
source:
  type: local
  path: .creed
targets:
  - name: codex
    enabled: true
    output_dir: .
config:
  - name: org
    path: config/org.md
`, map[string]string{
		".creed/config/org.md": "# Org rules\n\nCommit style: conventional.\n",
	})
	root := newLayeredConsumer(t, remote, `config:
  - name: repo
    path: config/repo.md
`, map[string]string{
		".creed/config/repo.md": "# Repo rules\n\nRun the focused tests first.\n",
	})

	result, err := New(root, WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Sync(context.Background(), usecase.SyncOptions{Target: "codex"})
	if err != nil {
		t.Fatalf("layered sync: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("layered sync target errors: %#v", result.Targets)
	}

	content := mustRead(t, filepath.Join(root, "AGENTS.md"))
	orgIndex := strings.Index(content, "# Org rules")
	repoIndex := strings.Index(content, "# Repo rules")
	if orgIndex < 0 || repoIndex < 0 || orgIndex >= repoIndex {
		t.Fatalf("layered AGENTS.md order = %q, want org content before repo content", content)
	}
	if !strings.Contains(content, "\n---\n") {
		t.Fatalf("layered AGENTS.md = %q, want separator", content)
	}
}

func TestLayeredDiffIsCleanForComposedOutputAndDetectsDrift(t *testing.T) {
	remote := newLayeredRemote(t, `version: 1
source:
  type: local
  path: .creed
config:
  - name: org
    path: config/org.md
`, map[string]string{
		".creed/config/org.md": "# Org rules\n",
	})
	root := newLayeredConsumer(t, remote, `targets:
  - name: codex
    enabled: true
    output_dir: .
config:
  - name: repo
    path: config/repo.md
`, map[string]string{
		".creed/config/repo.md": "# Repo rules\n",
		"AGENTS.md":             "# Org rules\n\n---\n\n# Repo rules\n",
	})

	ctx := context.Background()
	clean, err := New(root, WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Diff(ctx, usecase.DiffOptions{Target: "codex"})
	if err != nil {
		t.Fatalf("clean layered diff: %v", err)
	}
	if clean.HasDifferences() {
		t.Fatalf("clean layered diff reported drift: %q", clean.UnifiedDiff())
	}

	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Org rules\n\n---\n\nchanged\n"), 0644); err != nil {
		t.Fatal(err)
	}
	drift, err := New(root, WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Diff(ctx, usecase.DiffOptions{Target: "codex"})
	if err != nil {
		t.Fatalf("drifted layered diff: %v", err)
	}
	if !drift.HasDifferences() || !strings.Contains(drift.UnifiedDiff(), "+# Repo rules") {
		t.Fatalf("drifted layered diff = %q, want repo-layer replacement", drift.UnifiedDiff())
	}
}

func TestLayeredValidateChecksRemoteLayerFiles(t *testing.T) {
	remote := newLayeredRemote(t, `version: 1
source:
  type: local
  path: .creed
config:
  - name: org
    path: config/missing.md
`, nil)
	root := newLayeredConsumer(t, remote, `config:
  - name: repo
    path: config/repo.md
`, map[string]string{
		".creed/config/repo.md": "# Repo rules\n",
	})

	result, err := New(root, WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Validate(context.Background())
	if err != nil {
		t.Fatalf("layered validate: %v", err)
	}
	if result.Valid || !hasDiagnostic(result.Errors, "missing_layer_source_file") {
		t.Fatalf("layered validation = %#v, want missing_layer_source_file", result)
	}
}

func TestLayeredDoctorUsesRemoteValidationPath(t *testing.T) {
	remote := newLayeredRemote(t, `version: 1
source:
  type: local
  path: .creed
config:
  - name: org
    path: config/missing.md
`, nil)
	root := newLayeredConsumer(t, remote, `config:
  - name: repo
    path: config/repo.md
`, map[string]string{
		".creed/config/repo.md": "# Repo rules\n",
	})

	report, err := New(root, WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Doctor(context.Background())
	if err != nil {
		t.Fatalf("layered doctor: %v", err)
	}
	if !hasDoctorCheck(report.Checks, "error", "missing_layer_source_file") {
		t.Fatalf("layered doctor checks = %#v, want missing_layer_source_file", report.Checks)
	}
}

func TestPullPersistsLayeredManifestWithoutClobberingLocalSource(t *testing.T) {
	remote := newLayeredRemote(t, `version: 1
source:
  type: local
  path: .creed
targets:
  - name: codex
    enabled: true
    output_dir: .
config:
  - name: org
    path: config/org.md
`, map[string]string{
		".creed/config/org.md": "# Org rules\n",
	})
	root := newLayeredConsumer(t, remote, `targets:
  - name: codex
    enabled: true
    output_dir: .
config:
  - name: repo
    path: config/repo.md
`, map[string]string{
		".creed/config/repo.md": "# Repo rules\n",
	})

	if err := New(root, WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Pull(context.Background(), remote); err != nil {
		t.Fatalf("layered pull: %v", err)
	}
	content := mustRead(t, filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(content, "# Org rules") || !strings.Contains(content, "# Repo rules") {
		t.Fatalf("pulled AGENTS.md = %q, want both layers", content)
	}
	if got := mustRead(t, filepath.Join(root, ".creed", "config", "repo.md")); got != "# Repo rules\n" {
		t.Fatalf("pull clobbered local source: %q", got)
	}
	manifest := mustRead(t, filepath.Join(root, ".creed", "manifest.yaml"))
	if !strings.Contains(manifest, "type: layered") || !strings.Contains(manifest, "remote:") {
		t.Fatalf("pull manifest = %q, want persisted layered source", manifest)
	}
}

func TestValidateRejectsSymlinkedManifest(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "manifest.yaml"), []byte("version: 1\nsource:\n  type: local\n  path: .creed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".creed"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "manifest.yaml"), filepath.Join(root, ".creed", "manifest.yaml")); err != nil {
		t.Fatal(err)
	}
	result, err := New(root).Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if result.Valid || !hasDiagnostic(result.Errors, "invalid_manifest") {
		t.Fatalf("Validate() followed symlinked manifest: %#v", result)
	}
}

func TestLayeredValidateChecksRemoteTargets(t *testing.T) {
	remote := newLayeredRemote(t, `version: 1
source:
  type: local
  path: .creed
targets:
  - name: codex
    enabled: true
    output_dir: ../outside
`, nil)
	root := newLayeredConsumer(t, remote, `targets: []
`, nil)
	result, err := New(root, WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if result.Valid || !hasDiagnostic(result.Errors, "unsafe_target_output_dir") {
		t.Fatalf("Validate() accepted remote output_dir traversal: %#v", result)
	}
}

func TestLayeredValidateRejectsDuplicateEntriesWithinRemoteLayer(t *testing.T) {
	remote := newLayeredRemote(t, `version: 1
source:
  type: local
  path: .creed
config:
  - name: shared
    path: config/one.md
  - name: shared
    path: config/two.md
`, map[string]string{
		".creed/config/one.md": "one\n",
		".creed/config/two.md": "two\n",
	})
	root := newLayeredConsumer(t, remote, `targets: []
`, nil)
	result, err := New(root, WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	if result.Valid || !hasDiagnostic(result.Errors, "duplicate_source_name") {
		t.Fatalf("Validate() accepted duplicate remote entry: %#v", result)
	}
}

func TestPullRejectsCredentialBearingRemoteVariants(t *testing.T) {
	for _, remote := range []string{
		" HTTPS://user:secret@example.com/org.git",
		"HTTPS://user:secret@example.com/org.git",
		"ftp://user:secret@example.com/org.git",
	} {
		t.Run(remote, func(t *testing.T) {
			if err := New(t.TempDir()).Pull(context.Background(), remote); err == nil || !strings.Contains(err.Error(), "embedded credentials") {
				t.Fatalf("Pull(%q) error = %v, want credential rejection", remote, err)
			}
		})
	}
}

func TestLayeredValidateChecksOverriddenRemoteEntry(t *testing.T) {
	remote := newLayeredRemote(t, `version: 1
source:
  type: local
  path: .creed
config:
  - name: shared
    path: config/missing.md
`, nil)
	root := newLayeredConsumer(t, remote, `config:
  - name: shared
    path: config/local.md
`, map[string]string{
		".creed/config/local.md": "# Local override\n",
	})
	result, err := New(root, WithCacheDir(filepath.Join(t.TempDir(), "cache"))).Validate(context.Background())
	if err != nil {
		t.Fatalf("layered validate: %v", err)
	}
	if !hasDiagnostic(result.Errors, "missing_layer_source_file") {
		t.Fatalf("validation skipped overridden remote entry: %#v", result)
	}
}

func TestPullRejectsCredentialBearingRemoteBeforeWritingManifest(t *testing.T) {
	root := t.TempDir()
	err := New(root).Pull(context.Background(), "https://user:secret@example.com/org.git")
	if err == nil || !strings.Contains(err.Error(), "embedded credentials") {
		t.Fatalf("Pull() error = %v, want embedded-credential rejection", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".creed", "manifest.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("Pull() wrote a manifest after rejecting credentials: %v", statErr)
	}
}

func TestPullPreservesExistingLayerPathAndRef(t *testing.T) {
	remoteManifest := `version: 1
source:
  type: local
  path: .creed
`
	remote := newLayeredRemote(t, remoteManifest, map[string]string{"context/manifest.yaml": remoteManifest})
	root := newLayeredConsumer(t, remote, `targets: []
`, nil)
	manifestPath := filepath.Join(root, ".creed", "manifest.yaml")
	manifest := mustRead(t, manifestPath)
	manifest = strings.Replace(manifest, "      path: .creed\n", "      path: context\n      ref: main\n", 1)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := New(root).Pull(context.Background(), remote); err != nil {
		t.Fatalf("Pull(): %v", err)
	}
	got := mustRead(t, manifestPath)
	if !strings.Contains(got, "path: context") || !strings.Contains(got, "ref: main") {
		t.Fatalf("Pull() discarded layer path/ref: %q", got)
	}
}

func newLayeredRemote(t *testing.T, manifest string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeLayeredFile(t, root, ".creed/manifest.yaml", manifest)
	for path, content := range files {
		writeLayeredFile(t, root, path, content)
	}
	initLayeredGitRepo(t, root)
	return root
}

func newLayeredConsumer(t *testing.T, remote, manifestSuffix string, files map[string]string) string {
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
`, remote)
	manifest += manifestSuffix
	writeLayeredFile(t, root, ".creed/manifest.yaml", manifest)
	for path, content := range files {
		writeLayeredFile(t, root, path, content)
	}
	return root
}

func writeLayeredFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func initLayeredGitRepo(t *testing.T, root string) {
	t.Helper()
	commands := [][]string{
		{"git", "init", "-b", "main", root},
		{"git", "-C", root, "config", "user.name", "Creed Layer Test"},
		{"git", "-C", root, "config", "user.email", "creed-layer@example.invalid"},
		{"git", "-C", root, "add", "."},
		{"git", "-C", root, "commit", "-m", "add layered fixture"},
	}
	for _, args := range commands {
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
}
