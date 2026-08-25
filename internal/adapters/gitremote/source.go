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
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
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

var errCacheMiss = errors.New("git remote cache miss")

// Source reads creed data from a remote git repository.
// It implements ports.SourceReader by cloning the repository to a directory
// and delegating reads to a LocalFS adapter.
type Source struct {
	// remoteURL is the git clone URL.
	remoteURL string
	// sourcePath is the source directory relative to the cloned repository.
	sourcePath string
	// ref optionally pins the clone to a branch, tag, or commit SHA.
	ref string
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

// SourceOptions configures a GitRemote source reader.
type SourceOptions struct {
	// RemoteURL is the git clone URL.
	RemoteURL string
	// SourcePath is the source directory relative to the cloned repository.
	SourcePath string
	// Ref optionally pins the clone to a branch, tag, or commit SHA.
	Ref string
	// Token is an optional HTTPS authentication token.
	Token string
	// CacheDir enables persistent clone caching when non-empty.
	CacheDir string
}

// NewSource creates a GitRemote source reader for the given remote URL.
// An optional token can be provided for private repository access.
// Clones go to a temp directory with no persistent caching.
func NewSource(remoteURL, token string) *Source {
	return NewSourceWithOptions(SourceOptions{RemoteURL: remoteURL, Token: token})
}

// NewSourceWithCache creates a GitRemote source reader with persistent commit
// caching. The cacheDir stores clone directories and SHA metadata so that
// subsequent reads on unchanged remote HEAD skip the clone entirely.
func NewSourceWithCache(remoteURL, token, cacheDir string) *Source {
	return NewSourceWithOptions(SourceOptions{RemoteURL: remoteURL, Token: token, CacheDir: cacheDir})
}

// NewSourceWithOptions creates a GitRemote source reader with an explicit
// source subdirectory, optional ref pin, authentication token, and cache.
func NewSourceWithOptions(options SourceOptions) *Source {
	sourcePath := options.SourcePath
	if strings.TrimSpace(sourcePath) == "" {
		sourcePath = ".creed"
	}
	return &Source{
		remoteURL:  options.RemoteURL,
		sourcePath: sourcePath,
		ref:        strings.TrimSpace(options.Ref),
		token:      options.Token,
		cacheDir:   options.CacheDir,
	}
}

// cacheKey returns a deterministic cache key derived from the remote URL.
func (s *Source) cacheKey() string {
	material := s.remoteURL
	if s.ref != "" {
		material += "\x00" + s.ref
	}
	h := sha256.Sum256([]byte(material))
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

func (s *Source) ensureCacheLayout(create bool) error {
	if s.cacheDir == "" {
		return nil
	}
	for _, path := range []string{s.cacheDir, filepath.Join(s.cacheDir, "clones"), filepath.Join(s.cacheDir, "refs")} {
		if err := ensureDirectoryNoSymlink(path, create); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirectoryNoSymlink(path string, create bool) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	current := volume + string(filepath.Separator)
	rest := strings.TrimPrefix(absolute, current)
	for _, part := range strings.Split(rest, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			if !create {
				return statErr
			}
			if err := os.Mkdir(current, 0755); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("cache component %q must be a non-symlink directory", current)
		}
	}
	return nil
}

// writeCache persists the current SHA and clone directory to the cache file.
func (s *Source) writeCache() error {
	if err := s.ensureCacheLayout(true); err != nil {
		return fmt.Errorf("prepare cache layout: %w", err)
	}
	entry := cacheEntry{
		SHA: s.cachedSHA,
		Dir: s.clonedDir,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cache entry: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.cacheFilePath()), ".creed-cache-*")
	if err != nil {
		return fmt.Errorf("create cache temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod cache temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write cache temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close cache temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.cacheFilePath()); err != nil {
		return fmt.Errorf("replace cache file: %w", err)
	}
	return nil
}

// InvalidateCache removes this remote's cached clone and metadata. It is safe
// to call when caching is disabled or before the remote has ever been read.
func (s *Source) InvalidateCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cacheDir == "" {
		s.clonedDir, s.cachedSHA, s.localSource, s.cloned = "", "", nil, false
		return nil
	}
	if err := s.ensureCacheLayout(false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.clonedDir, s.cachedSHA, s.localSource, s.cloned = "", "", nil, false
			return nil
		}
		return fmt.Errorf("validate cache layout: %w", err)
	}
	if err := os.RemoveAll(s.clonePath()); err != nil {
		return fmt.Errorf("remove cached clone: %w", err)
	}
	if err := os.Remove(s.cacheFilePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove cache metadata: %w", err)
	}
	s.clonedDir, s.cachedSHA, s.localSource, s.cloned = "", "", nil, false
	return nil
}

// readCache reads the cache entry for this remote, if it exists.
func (s *Source) readCache() (*cacheEntry, error) {
	if err := s.ensureCacheLayout(false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: cache metadata missing", errCacheMiss)
		}
		return nil, err
	}
	if info, err := os.Lstat(s.cacheFilePath()); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: cache metadata is a symlink", errCacheMiss)
	}
	data, err := os.ReadFile(s.cacheFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: cache metadata missing", errCacheMiss)
		}
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
	refs, err := s.remoteRefs(ctx)
	if err != nil {
		return "", err
	}
	if s.ref != "" {
		return resolveRemoteRef(refs, s.ref)
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

	for _, branch := range []string{"main", "master"} {
		for _, ref := range refs {
			if ref.Name().IsBranch() && ref.Name().Short() == branch {
				return ref.Hash().String(), nil
			}
		}
	}
	return "", fmt.Errorf("no HEAD, master, or main reference found in remote")
}

func (s *Source) remoteRefs(ctx context.Context) ([]*plumbing.Reference, error) {
	repo, err := git.Init(memory.NewStorage(), nil)
	if err != nil {
		return nil, fmt.Errorf("init temp repo for ls-remote: %w", err)
	}
	remote, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{s.remoteURL}})
	if err != nil {
		return nil, fmt.Errorf("create remote: %w", err)
	}
	auth, err := s.authMethod()
	if err != nil {
		return nil, err
	}
	refs, err := remote.ListContext(ctx, &git.ListOptions{Auth: auth})
	if err != nil {
		return nil, classifyGitError("list remote refs", s.remoteURL, err)
	}
	return refs, nil
}

func resolveRemoteRef(refs []*plumbing.Reference, requested string) (string, error) {
	if isCommitSHA(requested) {
		return strings.ToLower(requested), nil
	}
	names := []string{}
	if strings.HasPrefix(requested, "refs/") {
		names = append(names, requested)
	} else {
		// Prefer branches when a name is ambiguous, then tags.
		names = append(names, plumbing.NewBranchReferenceName(requested).String(), plumbing.NewTagReferenceName(requested).String())
	}
	for _, name := range names {
		var direct *plumbing.Reference
		for _, ref := range refs {
			if ref.Name().String() == name {
				direct = ref
				break
			}
		}
		if direct == nil {
			continue
		}
		// Annotated tags may have a peeled ^{} ref. Prefer the commit hash.
		peeledName := plumbing.ReferenceName(name + "^{}").String()
		for _, ref := range refs {
			if ref.Name().String() == peeledName {
				return ref.Hash().String(), nil
			}
		}
		return direct.Hash().String(), nil
	}
	return "", fmt.Errorf("reference %q not found in remote", requested)
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
		} else if !errors.Is(err, errCacheMiss) {
			return err
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

	// Cache metadata is untrusted. Only accept the clone path owned by this
	// Source instance and reject symlinked/escaped cache directories.
	expectedDir := filepath.Clean(s.clonePath())
	if filepath.Clean(entry.Dir) != expectedDir {
		return fmt.Errorf("%w: cached clone path is outside the expected cache", errCacheMiss)
	}
	cacheRoot, err := filepath.EvalSymlinks(s.cacheDir)
	if err != nil {
		return fmt.Errorf("%w: cache root unavailable", errCacheMiss)
	}
	resolvedDir, err := filepath.EvalSymlinks(expectedDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: cached clone dir missing", errCacheMiss)
		}
		return fmt.Errorf("%w: cached clone cannot be resolved", errCacheMiss)
	}
	if relative, relErr := filepath.Rel(cacheRoot, resolvedDir); relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: cached clone escapes cache root", errCacheMiss)
	}
	if info, statErr := os.Lstat(expectedDir); statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: cached clone must be a non-symlink directory", errCacheMiss)
	}

	if isCommitSHA(s.ref) {
		if !strings.EqualFold(entry.SHA, s.ref) {
			return fmt.Errorf("%w: cached SHA does not match pinned ref", errCacheMiss)
		}
	} else {
		// Query remote HEAD or the mutable branch/tag ref to see if it changed.
		remoteSHA, err := s.remoteHeadSHA(ctx)
		if err != nil {
			return fmt.Errorf("cannot determine remote HEAD: %w", err)
		}
		if !strings.EqualFold(remoteSHA, entry.SHA) {
			return fmt.Errorf("%w: remote HEAD changed (was %s, now %s)", errCacheMiss, shortSHA(entry.SHA), shortSHA(remoteSHA))
		}
	}

	// Cache hit — reuse the existing clone directory.
	s.clonedDir = entry.Dir
	s.cachedSHA = entry.SHA
	s.localSource = localfs.NewSourceWithPath(entry.Dir, s.sourcePath)
	s.cloned = true
	return nil
}

// clone performs the actual git clone to a directory.
func (s *Source) clone(ctx context.Context) error {
	auth, err := s.authMethod()
	if err != nil {
		return err
	}

	var cloneDir string

	if s.cacheDir != "" {
		// Persistent clone directory for caching.
		if err := s.ensureCacheLayout(true); err != nil {
			return fmt.Errorf("prepare cache layout: %w", err)
		}
		cloneDir = s.clonePath()
		if info, statErr := os.Lstat(cloneDir); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("cached clone path must not be a symlink")
		}
		// Remove any stale clone from a previous run.
		if err := os.RemoveAll(cloneDir); err != nil {
			return fmt.Errorf("remove stale clone: %w", err)
		}
		if err := os.Mkdir(cloneDir, 0755); err != nil {
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
		Auth:  auth,
	}
	var checkoutSHA string
	if s.ref != "" {
		checkoutSHA, err = s.remoteHeadSHA(ctx)
		if err != nil {
			os.RemoveAll(cloneDir)
			return err
		}
		// Fetch the complete ref set so branch, lightweight-tag, annotated-tag,
		// and full-SHA pins all resolve to the same checked-out commit.
		cloneOpts.Tags = git.AllTags
		cloneOpts.Depth = 0
	}

	repo, err := git.PlainCloneContext(ctx, cloneDir, false, cloneOpts)
	if err != nil {
		// Clean up the failed clone directory.
		os.RemoveAll(cloneDir)
		return classifyGitError("clone repository", s.remoteURL, err)
	}

	if s.ref != "" {
		worktree, err := repo.Worktree()
		if err != nil {
			os.RemoveAll(cloneDir)
			return fmt.Errorf("open cloned worktree: %w", err)
		}
		checkout := &git.CheckoutOptions{Hash: plumbing.NewHash(checkoutSHA)}
		if err := worktree.Checkout(checkout); err != nil {
			os.RemoveAll(cloneDir)
			return classifyGitError("checkout reference", s.remoteURL, err)
		}
	}

	head, err := repo.Head()
	if err != nil {
		os.RemoveAll(cloneDir)
		return fmt.Errorf("get HEAD reference: %w", err)
	}

	s.clonedDir = cloneDir
	s.cachedSHA = head.Hash().String()
	s.localSource = localfs.NewSourceWithPath(cloneDir, s.sourcePath)
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

// authMethod returns the go-git auth method for the configured remote. HTTPS
// remotes use the explicit token passed to NewSource/NewSourceWithCache. SSH
// remotes prefer CREED_GIT_SSH_KEY when set, then fall back to SSH_AUTH_SOCK.
func (s *Source) authMethod() (transport.AuthMethod, error) {
	if s.token != "" && isHTTPSRemote(s.remoteURL) {
		return &http.BasicAuth{Username: "x-access-token", Password: s.token}, nil
	}
	if !isSSHRemote(s.remoteURL) {
		return nil, nil
	}
	if keyPath := os.Getenv("CREED_GIT_SSH_KEY"); keyPath != "" {
		method, err := ssh.NewPublicKeysFromFile("git", keyPath, os.Getenv("CREED_GIT_SSH_PASSPHRASE"))
		if err != nil {
			return nil, fmt.Errorf("load SSH key %q for git remote auth: %w", keyPath, err)
		}
		return method, nil
	}
	method, err := ssh.NewSSHAgentAuth("git")
	if err != nil {
		return nil, fmt.Errorf("SSH remote %q requires SSH auth: set SSH_AUTH_SOCK or CREED_GIT_SSH_KEY: %w", sanitizeRemoteURL(s.remoteURL), err)
	}
	return method, nil
}

func isHTTPSRemote(remoteURL string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(remoteURL)), "https://")
}

func isSSHRemote(remoteURL string) bool {
	remoteURL = strings.ToLower(strings.TrimSpace(remoteURL))
	return strings.HasPrefix(remoteURL, "git@") || strings.HasPrefix(remoteURL, "ssh://")
}

func isCommitSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, r := range ref {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && !(r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func classifyGitError(operation, remoteURL string, err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	detail := sanitizeErrorMessage(err.Error())
	sanitized := sanitizeRemoteURL(remoteURL)
	switch {
	case strings.Contains(msg, "authentication") || strings.Contains(msg, "authorization") || strings.Contains(msg, "permission denied") || strings.Contains(msg, "auth"):
		return fmt.Errorf("%s %q failed: authentication failed: %s", operation, sanitized, detail)
	case strings.Contains(msg, "repository not found") || strings.Contains(msg, "not found"):
		return fmt.Errorf("%s %q failed: repository not found: %s", operation, sanitized, detail)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s %q failed: context canceled or timed out: %s", operation, sanitized, detail)
	default:
		return fmt.Errorf("%s %q failed: %s", operation, sanitized, detail)
	}
}

func sanitizeErrorMessage(message string) string {
	fields := strings.Fields(message)
	for i, field := range fields {
		fields[i] = sanitizeRemoteURL(field)
	}
	return strings.Join(fields, " ")
}

func sanitizeRemoteURL(remoteURL string) string {
	remoteURL = strings.TrimSpace(remoteURL)
	parsed, err := url.Parse(remoteURL)
	if err == nil && parsed.Scheme != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	if at := strings.LastIndex(remoteURL, "@"); at >= 0 {
		if colon := strings.Index(remoteURL, ":"); colon > 0 && colon < at {
			return remoteURL[at+1:]
		}
	}
	return remoteURL
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
