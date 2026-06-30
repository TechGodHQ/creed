// Package gitremote implements the GitRemote source adapter for reading creed
// data from a remote git repository. It clones the repository to a persistent
// cache directory on first access and caches the last-pulled commit SHA to skip
// redundant clones on subsequent reads when the remote HEAD is unchanged.
package gitremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/techgodhq/creed/internal/adapters/localfs"
	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

// Compile-time assertion that Source implements ports.SourceReader.
var _ ports.SourceReader = (*Source)(nil)

// cacheEntry is the on-disk JSON representation of a cached clone.
type cacheEntry struct {
	SHA string `json:"sha"`
	Dir string `json:"dir"`
}

// Source reads creed data from a remote git repository.
// It implements ports.SourceReader by cloning the repository to a directory
// and delegating reads to a LocalFS adapter.
type Source struct {
	// remoteURL is the git clone URL (HTTPS).
	remoteURL string
	// token is an optional authentication token for private repos.
	token string
	// cacheDir is an optional persistent directory for commit-cache behavior.
	// When set, clones are cached and reused across Source instances if the
	// remote HEAD has not changed. When empty, each Source clones to a fresh
	// temp directory with no persistence.
	cacheDir string

	mu          sync.Mutex
	localSource *localfs.Source // delegate after clone
	clonedDir   string          // directory holding the clone
	cachedSHA   string          // last-pulled commit SHA
	cloned      bool            // whether the repo has been cloned in this instance
	cloneCount  int             // test hook: number of actual clone operations performed
}

// NewSource creates a GitRemote source reader for the given remote URL.
// An optional token can be provided for private repository access.
// Clones go to a temp directory with no persistent caching.
func NewSource(remoteURL, token string) *Source {
	return &Source{
		remoteURL: remoteURL,
		token:     token,
	}
}

// NewSourceWithCache creates a GitRemote source reader with persistent commit
// caching. The cacheDir stores clone directories and SHA metadata so that
// subsequent reads on unchanged remote HEAD skip the clone entirely.
func NewSourceWithCache(remoteURL, token, cacheDir string) *Source {
	return &Source{
		remoteURL: remoteURL,
		token:     token,
		cacheDir:  cacheDir,
	}
}

// cacheKey returns a deterministic cache key derived from the remote URL.
func (s *Source) cacheKey() string {
	h := sha256.Sum256([]byte(s.remoteURL))
	return hex.EncodeToString(h[:])
}

// clonePath returns the persistent directory for a cached clone.
func (s *Source) clonePath() string {
	return filepath.Join(s.cacheDir, "clones", s.cacheKey())
}

// cacheFilePath returns the path to the SHA metadata file for this remote.
func (s *Source) cacheFilePath() string {
	return filepath.Join(s.cacheDir, "refs", s.cacheKey()+".json")
}

// writeCache persists the current SHA and clone directory to the cache file.
func (s *Source) writeCache() error {
	entry := cacheEntry{
		SHA: s.cachedSHA,
		Dir: s.clonedDir,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cache entry: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.cacheFilePath()), 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	if err := os.WriteFile(s.cacheFilePath(), data, 0644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}
	return nil
}

// readCache reads the cache entry for this remote, if it exists.
func (s *Source) readCache() (*cacheEntry, error) {
	data, err := os.ReadFile(s.cacheFilePath())
	if err != nil {
		return nil, err
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// remoteHeadSHA queries the remote repository for the current HEAD commit SHA
// without cloning. Uses go-git's ls-remote via an in-memory repository.
func (s *Source) remoteHeadSHA(ctx context.Context) (string, error) {
	repo, err := git.Init(memory.NewStorage(), nil)
	if err != nil {
		return "", fmt.Errorf("init temp repo for ls-remote: %w", err)
	}

	remoteCfg := &config.RemoteConfig{
		Name: "origin",
		URLs: []string{s.remoteURL},
	}

	remote, err := repo.CreateRemote(remoteCfg)
	if err != nil {
		return "", fmt.Errorf("create remote: %w", err)
	}

	listOpts := &git.ListOptions{}
	if s.token != "" {
		listOpts.Auth = &http.BasicAuth{
			Username: "x-access-token",
			Password: s.token,
		}
	}

	refs, err := remote.ListContext(ctx, listOpts)
	if err != nil {
		return "", fmt.Errorf("list remote refs: %w", err)
	}

	// Prefer the HEAD reference. In ls-remote output, HEAD may be a
	// symbolic ref with a zero hash (common for local repos). In that
	// case, fall through to branch resolution below.
	for _, ref := range refs {
		if ref.Name() == plumbing.HEAD {
			if !ref.Hash().IsZero() {
				return ref.Hash().String(), nil
			}
			break
		}
	}

	// Fall back to main branch.
	for _, ref := range refs {
		if ref.Name().IsBranch() && ref.Name().Short() == "main" {
			return ref.Hash().String(), nil
		}
	}

	// Fall back to master branch.
	for _, ref := range refs {
		if ref.Name().IsBranch() && ref.Name().Short() == "master" {
			return ref.Hash().String(), nil
		}
	}

	return "", fmt.Errorf("no HEAD, master, or main reference found in remote")
}

// ensureCloned clones the repository on first access. Subsequent calls within
// the same Source instance are no-ops. When persistent caching is enabled,
// the method checks the cache and remote HEAD before cloning.
func (s *Source) ensureCloned(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cloned {
		return nil
	}

	// Try to reuse a cached clone when the remote HEAD is unchanged.
	if s.cacheDir != "" {
		if err := s.tryCache(ctx); err == nil {
			return nil // Cache hit — clone skipped.
		}
	}

	// No cache or cache miss — clone fresh.
	s.cloneCount++

	if err := s.clone(ctx); err != nil {
		return err
	}

	// Persist cache entry for future reuse.
	if s.cacheDir != "" {
		// Best-effort: cache write failure doesn't block the read.
		_ = s.writeCache()
	}

	return nil
}

// tryCache attempts to reuse a cached clone directory. Returns nil on success
// (cache hit), or an error if the cache is missing, stale, or the cached
// directory no longer exists.
func (s *Source) tryCache(ctx context.Context) error {
	entry, err := s.readCache()
	if err != nil {
		return err
	}

	// Check if the cached clone directory still exists.
	if _, err := os.Stat(entry.Dir); err != nil {
		return fmt.Errorf("cached clone dir missing: %w", err)
	}

	// Query remote HEAD to see if it has changed.
	remoteSHA, err := s.remoteHeadSHA(ctx)
	if err != nil {
		return fmt.Errorf("cannot determine remote HEAD: %w", err)
	}

	if remoteSHA != entry.SHA {
		return fmt.Errorf("remote HEAD changed (was %s, now %s)", shortSHA(entry.SHA), shortSHA(remoteSHA))
	}

	// Cache hit — reuse the existing clone directory.
	s.clonedDir = entry.Dir
	s.cachedSHA = entry.SHA
	s.localSource = localfs.NewSource(entry.Dir)
	s.cloned = true
	return nil
}

// clone performs the actual git clone to a directory.
func (s *Source) clone(ctx context.Context) error {
	var cloneDir string

	if s.cacheDir != "" {
		// Persistent clone directory for caching.
		cloneDir = s.clonePath()
		// Remove any stale clone from a previous run.
		os.RemoveAll(cloneDir)
		if err := os.MkdirAll(cloneDir, 0755); err != nil {
			return fmt.Errorf("create clone dir: %w", err)
		}
	} else {
		// Temp directory for one-off clones.
		tmpDir, err := os.MkdirTemp("", "creed-clone-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		cloneDir = tmpDir
	}

	cloneOpts := &git.CloneOptions{
		URL:   s.remoteURL,
		Tags:  git.NoTags,
		Depth: 1,
	}
	if s.token != "" {
		cloneOpts.URL = injectToken(s.remoteURL, s.token)
	}

	repo, err := git.PlainCloneContext(ctx, cloneDir, false, cloneOpts)
	if err != nil {
		// Clean up the failed clone directory.
		os.RemoveAll(cloneDir)
		return fmt.Errorf("clone repository: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		os.RemoveAll(cloneDir)
		return fmt.Errorf("get HEAD reference: %w", err)
	}

	s.clonedDir = cloneDir
	s.cachedSHA = head.Hash().String()
	s.localSource = localfs.NewSource(cloneDir)
	s.cloned = true

	return nil
}

// CachedSHA returns the last-cloned commit SHA, or empty string if not yet cloned.
func (s *Source) CachedSHA() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cachedSHA
}

// CloneCount returns the number of actual clone operations performed by this
// Source instance. Used for testing the commit-cache behavior.
func (s *Source) CloneCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cloneCount
}

// Cleanup removes the clone directory. For cached clones, the directory is
// NOT removed (it persists for reuse). Only temp-directory clones are cleaned.
func (s *Source) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cacheDir != "" {
		// Cached clones persist — do not remove.
		return nil
	}

	if s.clonedDir != "" {
		err := os.RemoveAll(s.clonedDir)
		s.clonedDir = ""
		s.cloned = false
		s.localSource = nil
		return err
	}
	return nil
}

// ReadManifest delegates to the LocalFS adapter after ensuring the repo is cloned.
func (s *Source) ReadManifest(ctx context.Context) (*domain.Manifest, error) {
	if err := s.ensureCloned(ctx); err != nil {
		return nil, err
	}
	return s.localSource.ReadManifest(ctx)
}

// ReadSkill delegates to the LocalFS adapter after ensuring the repo is cloned.
func (s *Source) ReadSkill(ctx context.Context, name string) (*domain.Skill, error) {
	if err := s.ensureCloned(ctx); err != nil {
		return nil, err
	}
	return s.localSource.ReadSkill(ctx, name)
}

// ListSkills delegates to the LocalFS adapter after ensuring the repo is cloned.
func (s *Source) ListSkills(ctx context.Context) ([]domain.SkillInfo, error) {
	if err := s.ensureCloned(ctx); err != nil {
		return nil, err
	}
	return s.localSource.ListSkills(ctx)
}

// ReadConfig delegates to the LocalFS adapter after ensuring the repo is cloned.
func (s *Source) ReadConfig(ctx context.Context, name string) (*domain.ConfigFile, error) {
	if err := s.ensureCloned(ctx); err != nil {
		return nil, err
	}
	return s.localSource.ReadConfig(ctx, name)
}

// ListConfigs delegates to the LocalFS adapter after ensuring the repo is cloned.
func (s *Source) ListConfigs(ctx context.Context) ([]domain.ConfigInfo, error) {
	if err := s.ensureCloned(ctx); err != nil {
		return nil, err
	}
	return s.localSource.ListConfigs(ctx)
}

// shortSHA returns the first 8 characters of a SHA string, or the full
// string if it's shorter than 8 characters. Safe for use on potentially
// truncated or corrupted cache data.
func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// injectToken injects an authentication token into an HTTPS git URL.
// For example: https://github.com/user/repo → https://x-access-token:TOKEN@github.com/user/repo
// Non-HTTPS URLs or empty tokens are returned unchanged.
func injectToken(rawURL, token string) string {
	if token == "" {
		return rawURL
	}
	// Only inject for HTTPS URLs.
	const httpsPrefix = "https://"
	if len(rawURL) <= len(httpsPrefix) || rawURL[:len(httpsPrefix)] != httpsPrefix {
		return rawURL
	}
	rest := rawURL[len(httpsPrefix):]
	return httpsPrefix + "x-access-token:" + token + "@" + rest
}

// CloneDir returns the path to the cloned repository directory (for testing).
// Returns empty string if not yet cloned.
func (s *Source) CloneDir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clonedDir
}
