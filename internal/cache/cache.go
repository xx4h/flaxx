// Package cache provides a simple file-based TTL cache for registry lookups.
//
// Entries are JSON files stored under the cache directory, keyed by a
// sha256 hash of the caller-provided key parts. Each file embeds its
// fetched_at timestamp so TTL checks are robust against filesystem mtime
// mutations. Writes are atomic via temp-file + rename.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cache is a file-based TTL cache. The zero value is not usable; call New.
//
// A nil *Cache is safe to call Get/Set on (both are no-ops) so callers can
// keep cache logic unconditional without nil-checks.
type Cache struct {
	dir        string
	ttl        time.Duration
	enabled    bool
	bypassRead bool
}

// entry is the on-disk representation of a cached value.
type entry struct {
	FetchedAt time.Time       `json:"fetched_at"`
	Payload   json.RawMessage `json:"payload"`
}

// New returns a Cache rooted at $XDG_CACHE_HOME/flaxx/registries (or the
// equivalent fallback). The directory is created lazily on first Set.
// If enabled is false, Get/Set become no-ops.
func New(ttl time.Duration, enabled bool) (*Cache, error) {
	dir, err := defaultDir()
	if err != nil {
		return nil, err
	}
	return &Cache{dir: dir, ttl: ttl, enabled: enabled}, nil
}

// NewAt returns a Cache rooted at the given directory. Primarily for tests.
func NewAt(dir string, ttl time.Duration, enabled bool) *Cache {
	return &Cache{dir: dir, ttl: ttl, enabled: enabled}
}

// WithBypassRead returns a shallow copy of c where Get always reports a miss.
// Set still writes through, so a bypassed run refreshes the cache for
// subsequent callers (used by `check --no-cache`).
func (c *Cache) WithBypassRead() *Cache {
	if c == nil {
		return nil
	}
	cp := *c
	cp.bypassRead = true
	return &cp
}

// Get looks up key and decodes the payload into out.
// Returns (true, nil) on a fresh hit; (false, nil) on miss/expired/disabled;
// (false, err) only for unexpected IO or decode errors on a present file.
func (c *Cache) Get(key string, out any) (bool, error) {
	if c == nil || !c.enabled || c.bypassRead {
		return false, nil
	}
	path := c.pathFor(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("reading cache %s: %w", path, err)
	}

	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		// Treat corrupted entries as misses — the caller will refetch
		// and overwrite.
		return false, nil
	}

	if c.ttl > 0 && time.Since(e.FetchedAt) > c.ttl {
		return false, nil
	}

	if err := json.Unmarshal(e.Payload, out); err != nil {
		return false, nil
	}
	return true, nil
}

// Set encodes val as JSON and writes it to the cache atomically.
// A nil or disabled cache is a no-op; errors are returned so callers
// can decide whether to log them.
func (c *Cache) Set(key string, val any) error {
	if c == nil || !c.enabled {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir %s: %w", c.dir, err)
	}

	payload, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshaling cache payload: %w", err)
	}
	data, err := json.Marshal(entry{FetchedAt: time.Now().UTC(), Payload: payload})
	if err != nil {
		return fmt.Errorf("marshaling cache entry: %w", err)
	}

	path := c.pathFor(key)
	tmp, err := os.CreateTemp(c.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing tempfile: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming cache file: %w", err)
	}
	return nil
}

func (c *Cache) pathFor(key string) string {
	return filepath.Join(c.dir, key+".json")
}

// Key joins parts with a separator and returns a sha256 hex digest safe for
// use as a filename. Callers supply stable, human-meaningful prefixes
// (e.g. "helm", "oci") to namespace entries.
func Key(parts ...string) string {
	joined := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

// defaultDir returns $XDG_CACHE_HOME/flaxx/registries or the OS-provided
// equivalent. We use os.UserCacheDir which already honors XDG_CACHE_HOME
// on Linux and maps to the right path on macOS/Windows.
func defaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving user cache dir: %w", err)
	}
	return filepath.Join(base, "flaxx", "registries"), nil
}
