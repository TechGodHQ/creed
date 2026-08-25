package gitremote

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// createBareRepo creates a bare git repo with a .creed/ manifest and skill,
// returning the path to the bare repo (usable as a remote URL).
func createBareRepo(t *testing.T) string {
	t.Helper()

	// Create a working directory, init the content, then push to a bare repo.
	workDir := t.TempDir()
	bareDir := t.TempDir() + "-bare.git"

	// Init bare repo.
	if err := exec.Command("git", "init", "--bare", bareDir).Run(); err != nil {
		t.Fatalf("git init --bare failed: %v", err)
	}

	// Create working repo with content.
	creedDir := filepath.Join(workDir, ".creed")
	if err := os.MkdirAll(filepath.Join(creedDir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := `version: 1
source:
  type: local
  path: .creed

targets:
  - name: claude
    enabled: true
    output_dir: .

skills:
  - name: code-review
    path: skills/code-review.md
`
	if err := os.WriteFile(filepath.Join(creedDir, "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creedDir, "skills", "code-review.md"), []byte("# Code Review"), 0644); err != nil {
		t.Fatal(err)
	}

	// Init and push.
	for _, args := range [][]string{
		{"git", "init", workDir},
		{"git", "-C", workDir, "config", "user.email", "test@test.com"},
		{"git", "-C", workDir, "config", "user.name", "Test"},
		{"git", "-C", workDir, "add", "-A"},
		{"git", "-C", workDir, "commit", "-m", "initial"},
		{"git", "-C", workDir, "remote", "add", "origin", bareDir},
		{"git", "-C", workDir, "push", "origin", "HEAD"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	return bareDir
}

func TestGitRemoteCloneAndReadManifest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}

	bareURL := createBareRepo(t)
	src := NewSource(bareURL, "")
	defer src.Cleanup()

	ctx := context.Background()
	m, err := src.ReadManifest(ctx)
	if err != nil {
		t.Fatalf("ReadManifest error: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("expected Version == 1, got %d", m.Version)
	}
	if len(m.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(m.Skills))
	}
	if m.Skills[0].Name != "code-review" {
		t.Errorf("expected skill name \"code-review\", got %q", m.Skills[0].Name)
	}
}

func TestGitRemoteReadSkill(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}

	bareURL := createBareRepo(t)
	src := NewSource(bareURL, "")
	defer src.Cleanup()

	skill, err := src.ReadSkill(context.Background(), "code-review")
	if err != nil {
		t.Fatalf("ReadSkill error: %v", err)
	}
	if string(skill.Content) != "# Code Review" {
		t.Errorf("unexpected content: %q", string(skill.Content))
	}
}

func TestGitRemoteCloneFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}

	src := NewSource("/nonexistent/path/to/repo.git", "")
	defer src.Cleanup()

	_, err := src.ReadManifest(context.Background())
	if err == nil {
		t.Fatal("expected error for unreachable repo, got nil")
	}
}

func TestGitRemoteCachedSHA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}

	bareURL := createBareRepo(t)
	src := NewSource(bareURL, "")
	defer src.Cleanup()

	// Before cloning, SHA should be empty.
	if sha := src.CachedSHA(); sha != "" {
		t.Errorf("expected empty SHA before clone, got %q", sha)
	}

	// Trigger clone.
	if _, err := src.ReadManifest(context.Background()); err != nil {
		t.Fatalf("ReadManifest error: %v", err)
	}

	// After cloning, SHA should be populated.
	if sha := src.CachedSHA(); sha == "" {
		t.Error("expected non-empty SHA after clone")
	}
}

func TestGitRemoteCommitCacheHit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}

	bareURL := createBareRepo(t)
	cacheDir := t.TempDir()

	// First source: should clone.
	src1 := NewSourceWithCache(bareURL, "", cacheDir)
	if _, err := src1.ReadManifest(context.Background()); err != nil {
		t.Fatalf("first ReadManifest error: %v", err)
	}
	if src1.CloneCount() != 1 {
		t.Errorf("first source: expected CloneCount 1, got %d", src1.CloneCount())
	}
	if src1.CachedSHA() == "" {
		t.Fatal("first source: expected non-empty SHA after clone")
	}

	// Second source with same cache dir: should reuse cached clone (no new clone).
	src2 := NewSourceWithCache(bareURL, "", cacheDir)
	if _, err := src2.ReadManifest(context.Background()); err != nil {
		t.Fatalf("second ReadManifest error: %v", err)
	}
	if src2.CloneCount() != 0 {
		t.Errorf("second source: expected CloneCount 0 (cache hit), got %d", src2.CloneCount())
	}
	if src2.CachedSHA() != src1.CachedSHA() {
		t.Errorf("SHA mismatch: src1=%s, src2=%s", src1.CachedSHA(), src2.CachedSHA())
	}

	// Verify the data is readable from the cached clone.
	m, err := src2.ReadManifest(context.Background())
	if err != nil {
		t.Fatalf("ReadManifest from cached clone error: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("expected Version 1, got %d", m.Version)
	}
}

func TestGitRemoteCommitCacheMissOnChangedRemote(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}

	bareURL := createBareRepo(t)
	cacheDir := t.TempDir()

	// First clone.
	src1 := NewSourceWithCache(bareURL, "", cacheDir)
	if _, err := src1.ReadManifest(context.Background()); err != nil {
		t.Fatalf("first ReadManifest error: %v", err)
	}
	if src1.CloneCount() != 1 {
		t.Fatalf("expected CloneCount 1, got %d", src1.CloneCount())
	}
	firstSHA := src1.CachedSHA()

	// Add a new commit to the remote repo so the HEAD changes.
	addCommitToBareRepo(t, bareURL)

	// Second source: remote HEAD changed, so cache should be invalidated.
	src2 := NewSourceWithCache(bareURL, "", cacheDir)
	if _, err := src2.ReadManifest(context.Background()); err != nil {
		t.Fatalf("second ReadManifest error: %v", err)
	}
	if src2.CloneCount() != 1 {
		t.Errorf("expected CloneCount 1 (re-clone after cache miss), got %d", src2.CloneCount())
	}
	if src2.CachedSHA() == firstSHA {
		t.Error("expected different SHA after remote change")
	}
}

func TestGitRemoteNoCacheAlwaysClones(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}

	bareURL := createBareRepo(t)

	// Without cache, each source clones fresh.
	src1 := NewSource(bareURL, "")
	defer src1.Cleanup()
	if _, err := src1.ReadManifest(context.Background()); err != nil {
		t.Fatalf("first ReadManifest error: %v", err)
	}
	if src1.CloneCount() != 1 {
		t.Errorf("first source: expected CloneCount 1, got %d", src1.CloneCount())
	}

	// Second source without cache — no persistence, must clone again.
	src2 := NewSource(bareURL, "")
	defer src2.Cleanup()
	if _, err := src2.ReadManifest(context.Background()); err != nil {
		t.Fatalf("second ReadManifest error: %v", err)
	}
	if src2.CloneCount() != 1 {
		t.Errorf("second source: expected CloneCount 1 (no cache), got %d", src2.CloneCount())
	}
}

func TestGitRemoteExplicitCacheInvalidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}

	bareURL := createBareRepo(t)
	cacheDir := t.TempDir()

	src1 := NewSourceWithCache(bareURL, "", cacheDir)
	if _, err := src1.ReadManifest(context.Background()); err != nil {
		t.Fatalf("first ReadManifest error: %v", err)
	}
	if src1.CloneCount() != 1 {
		t.Fatalf("expected first source CloneCount 1, got %d", src1.CloneCount())
	}
	if err := src1.InvalidateCache(); err != nil {
		t.Fatalf("InvalidateCache error: %v", err)
	}

	src2 := NewSourceWithCache(bareURL, "", cacheDir)
	if _, err := src2.ReadManifest(context.Background()); err != nil {
		t.Fatalf("second ReadManifest error: %v", err)
	}
	if src2.CloneCount() != 1 {
		t.Fatalf("expected clone after explicit invalidation, got %d", src2.CloneCount())
	}
}

func TestGitRemoteRealPublicRemote(t *testing.T) {
	if os.Getenv("CREED_RUN_REMOTE_INTEGRATION") != "1" {
		t.Skip("set CREED_RUN_REMOTE_INTEGRATION=1 to clone the public Creed repo")
	}

	src := NewSource("https://github.com/TechGodHQ/creed.git", "")
	defer src.Cleanup()
	manifest, err := src.ReadManifest(context.Background())
	if err != nil {
		t.Fatalf("ReadManifest from public remote error: %v", err)
	}
	if manifest.Version != 1 {
		t.Fatalf("manifest version = %d, want 1", manifest.Version)
	}
}

// addCommitToBareRepo adds a new commit to the working tree and pushes
// to the bare repo, changing the remote HEAD SHA.
func addCommitToBareRepo(t *testing.T, bareURL string) {
	t.Helper()
	workDir := t.TempDir()

	// Clone the bare repo, add a file, commit, push back.
	for _, args := range [][]string{
		{"git", "clone", bareURL, workDir},
		{"git", "-C", workDir, "config", "user.email", "test@test.com"},
		{"git", "-C", workDir, "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	// Add a new file.
	newFile := filepath.Join(workDir, "CHANGELOG.md")
	if err := os.WriteFile(newFile, []byte("# Changelog\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"git", "-C", workDir, "add", "-A"},
		{"git", "-C", workDir, "commit", "-m", "add changelog"},
		{"git", "-C", workDir, "push", "origin", "HEAD"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}
}

func TestInjectToken(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		token   string
		wantURL string
	}{
		{
			name:    "HTTPS URL gets token",
			url:     "https://github.com/user/repo",
			token:   "abc123",
			wantURL: "https://x-access-token:abc123@github.com/user/repo",
		},
		{
			name:    "SSH URL unchanged",
			url:     "git@github.com:user/repo.git",
			token:   "abc123",
			wantURL: "git@github.com:user/repo.git",
		},
		{
			name:    "no token unchanged",
			url:     "https://github.com/user/repo",
			token:   "",
			wantURL: "https://github.com/user/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injectToken(tt.url, tt.token)
			if got != tt.wantURL {
				t.Errorf("injectToken(%q, %q) = %q, want %q", tt.url, tt.token, got, tt.wantURL)
			}
		})
	}
}

func TestAuthMethodUsesHTTPTokenWithoutInjectingURL(t *testing.T) {
	src := NewSource("https://github.com/user/repo.git", "token-123")
	auth, err := src.authMethod()
	if err != nil {
		t.Fatalf("authMethod returned error: %v", err)
	}
	basic, ok := auth.(*http.BasicAuth)
	if !ok {
		t.Fatalf("authMethod type = %T, want *http.BasicAuth", auth)
	}
	if basic.Username != "x-access-token" || basic.Password != "token-123" {
		t.Fatalf("unexpected basic auth: %#v", basic)
	}
}

func TestAuthMethodRejectsSSHWithoutAgentOrKey(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("CREED_GIT_SSH_KEY", "")
	src := NewSource("git@github.com:user/private.git", "")
	_, err := src.authMethod()
	if err == nil {
		t.Fatal("expected SSH auth setup error, got nil")
	}
	if !strings.Contains(err.Error(), "SSH_AUTH_SOCK") || !strings.Contains(err.Error(), "CREED_GIT_SSH_KEY") {
		t.Fatalf("error %q does not mention SSH auth options", err)
	}
}

func TestClassifyGitErrorSanitizesRemoteURL(t *testing.T) {
	err := classifyGitError(
		"clone repository",
		"https://x-access-token:secret-token@github.com/user/repo.git",
		errors.New("authentication required for https://x-access-token:secret-token@github.com/user/repo.git"),
	)
	if err == nil {
		t.Fatal("expected classified error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("classified error leaked token: %v", err)
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("classified error did not label auth failure: %v", err)
	}
}

func TestGitRemotePinnedCommitCacheWorksOffline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}
	bareURL := createBareRepo(t)
	output, err := exec.Command("git", "--git-dir", bareURL, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse: %v: %s", err, output)
	}
	ref := strings.TrimSpace(string(output))
	cacheDir := t.TempDir()
	options := SourceOptions{RemoteURL: bareURL, Ref: ref, CacheDir: cacheDir}
	first := NewSourceWithOptions(options)
	if _, err := first.ReadManifest(context.Background()); err != nil {
		t.Fatalf("first pinned read: %v", err)
	}
	if first.CachedSHA() != ref {
		t.Fatalf("first pinned SHA = %q, want %q", first.CachedSHA(), ref)
	}
	if err := os.RemoveAll(bareURL); err != nil {
		t.Fatalf("remove remote: %v", err)
	}
	second := NewSourceWithOptions(options)
	if _, err := second.ReadManifest(context.Background()); err != nil {
		t.Fatalf("offline pinned cache read: %v", err)
	}
	if second.CloneCount() != 0 {
		t.Fatalf("offline pinned cache cloned %d times, want 0", second.CloneCount())
	}
}

func TestGitRemoteAnnotatedTagRef(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}
	bareURL := createBareRepo(t)
	workDir := t.TempDir()
	commands := [][]string{
		{"git", "clone", bareURL, workDir},
		{"git", "-C", workDir, "config", "user.name", "Tag Test"},
		{"git", "-C", workDir, "config", "user.email", "tag@example.invalid"},
		{"git", "-C", workDir, "tag", "-a", "v1", "-m", "release"},
		{"git", "-C", workDir, "push", "origin", "refs/tags/v1"},
	}
	for _, args := range commands {
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	src := NewSourceWithOptions(SourceOptions{RemoteURL: bareURL, Ref: "v1"})
	if _, err := src.ReadManifest(context.Background()); err != nil {
		t.Fatalf("annotated tag read: %v", err)
	}
	defer src.Cleanup()
}

func TestGitRemoteRejectsEscapedCacheMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}
	bareURL := createBareRepo(t)
	cacheDir := t.TempDir()
	src := NewSourceWithCache(bareURL, "", cacheDir)
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(external, ".creed"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, ".creed", "manifest.yaml"), []byte("version: 1\nsource:\n  type: local\n  path: .creed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(src.cacheFilePath()), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cacheEntry{SHA: "0123456789012345678901234567890123456789", Dir: external})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src.cacheFilePath(), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(bareURL); err != nil {
		t.Fatal(err)
	}
	if _, err := src.ReadManifest(context.Background()); err == nil {
		t.Fatal("ReadManifest trusted cache metadata outside the configured cache")
	}
}

func TestGitRemoteNonDefaultBranchAndSHARefs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in short mode")
	}
	bareURL := createBareRepo(t)
	workDir := t.TempDir()
	for _, args := range [][]string{
		{"git", "clone", bareURL, workDir},
		{"git", "-C", workDir, "config", "user.name", "Branch Test"},
		{"git", "-C", workDir, "config", "user.email", "branch@example.invalid"},
		{"git", "-C", workDir, "checkout", "-b", "feature-only"},
	} {
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	featureSkill := filepath.Join(workDir, ".creed", "skills", "code-review.md")
	if err := os.WriteFile(featureSkill, []byte("# Feature-only branch"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "-C", workDir, "add", ".creed/skills/code-review.md"},
		{"git", "-C", workDir, "commit", "-m", "feature-only context"},
		{"git", "-C", workDir, "push", "origin", "HEAD:refs/heads/feature-only"},
	} {
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	output, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse: %v: %s", err, output)
	}
	sha := strings.TrimSpace(string(output))
	for _, ref := range []string{"feature-only", sha} {
		src := NewSourceWithOptions(SourceOptions{RemoteURL: bareURL, Ref: ref})
		skill, readErr := src.ReadSkill(context.Background(), "code-review")
		if readErr != nil {
			t.Fatalf("read ref %q: %v", ref, readErr)
		}
		if string(skill.Content) != "# Feature-only branch" {
			t.Fatalf("ref %q content = %q, want feature-only branch", ref, skill.Content)
		}
		if cleanupErr := src.Cleanup(); cleanupErr != nil {
			t.Fatalf("cleanup ref %q: %v", ref, cleanupErr)
		}
	}
}
