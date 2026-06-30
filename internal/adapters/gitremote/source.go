// Package gitremote implements the GitRemote source adapter for reading creed
// data from a remote git repository. It clones the repository to a temporary
// directory on first access and caches the last-pulled commit SHA to skip
// redundant clones on subsequent reads.
package gitremote

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/go-git/go-git/v5"

	"github.com/techgodhq/creed/internal/adapters/localfs"
	"github.com/techgodhq/creed/internal/domain"
)

// Source reads creed data from a remote git repository.
// It implements ports.SourceReader by cloning the repository to a temp
// directory and delegating reads to a LocalFS adapter.
type Source struct {
	// remoteURL is the git clone URL (HTTPS).
	remoteURL string
	// token is an optional authentication token for private repos.
	token string

	mu          sync.Mutex
	localSource *localfs.Source // delegate after clone
	clonedDir   string          // temp directory holding the clone
	cachedSHA   string          // last-pulled commit SHA
	cloned      bool            // whether the repo has been cloned in this instance
}

// NewSource creates a GitRemote source reader for the given remote URL.
// An optional token can be provided for private repository access.
func NewSource(remoteURL, token string) *Source {
	return &Source{
		remoteURL: remoteURL,
		token:     token,
	}
}

// ensureCloned clones the repository on first access. Subsequent calls
// reuse the existing clone and update the cached SHA.
func (s *Source) ensureCloned(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cloned {
		return nil
	}

	// Create a temp directory for the clone.
	tmpDir, err := os.MkdirTemp("", "creed-clone-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Clone options.
	cloneOpts := &git.CloneOptions{
		URL:  s.remoteURL,
		Tags: git.NoTags,
	}
	if s.token != "" {
		// For HTTPS URLs with token, inject into the URL.
		// go-git handles basic auth via transport, but for simplicity
		// we embed the token in the clone URL when provided.
		cloneOpts.URL = injectToken(s.remoteURL, s.token)
	}

	// Clone with depth 1 to minimize transfer.
	cloneOpts.Depth = 1

	repo, err := git.PlainCloneContext(ctx, tmpDir, false, cloneOpts)
	if err != nil {
		// Clean up the partial clone on failure.
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Cache the HEAD commit SHA.
	head, err := repo.Head()
	if err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("failed to get HEAD reference: %w", err)
	}
	s.cachedSHA = head.Hash().String()

	s.clonedDir = tmpDir
	s.localSource = localfs.NewSource(tmpDir)
	s.cloned = true

	return nil
}

// CachedSHA returns the last-cloned commit SHA, or empty string if not yet cloned.
func (s *Source) CachedSHA() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cachedSHA
}

// Cleanup removes the temporary clone directory. Should be called when the
// source is no longer needed.
func (s *Source) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

// injectToken injects an authentication token into an HTTPS git URL.
// For example: https://github.com/user/repo → https://x-access-token:TOKEN@github.com/user/repo
// Non-HTTPS URLs are returned unchanged.
func injectToken(rawURL, token string) string {
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
