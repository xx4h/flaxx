package checker

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xx4h/flaxx/internal/cache"
)

// TestCachedFetchTags_HitsCacheOnSecondCall verifies that installing an
// activeCache causes the second call for the same registry+repo to skip
// the HTTP fetcher entirely.
func TestCachedFetchTags_HitsCacheOnSecondCall(t *testing.T) {
	var calls int32
	origFetch := fetchTagsFunc
	fetchTagsFunc = func(_ *http.Client, _, _ string) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"1.0.0", "1.1.0", "2.0.0"}, nil
	}
	defer func() { fetchTagsFunc = origFetch }()

	SetCache(cache.NewAt(t.TempDir(), time.Hour, true))
	defer SetCache(nil)

	if _, err := cachedFetchTags("reg.example.com", "org/app"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := cachedFetchTags("reg.example.com", "org/app"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected 1 upstream fetch with caching enabled, got %d", got)
	}
}

// TestCachedFetchTags_BypassReadStillWrites verifies --no-cache semantics:
// we always fetch fresh, but the cache is refreshed so subsequent non-bypass
// callers see the new value.
func TestCachedFetchTags_BypassReadStillWrites(t *testing.T) {
	var calls int32
	origFetch := fetchTagsFunc
	fetchTagsFunc = func(_ *http.Client, _, _ string) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"v1"}, nil
	}
	defer func() { fetchTagsFunc = origFetch }()

	c := cache.NewAt(t.TempDir(), time.Hour, true)

	SetCache(c)
	if _, err := cachedFetchTags("reg", "repo"); err != nil {
		t.Fatal(err)
	}

	SetCache(c.WithBypassRead())
	if _, err := cachedFetchTags("reg", "repo"); err != nil {
		t.Fatal(err)
	}

	SetCache(c) // back to normal — should now be a hit
	if _, err := cachedFetchTags("reg", "repo"); err != nil {
		t.Fatal(err)
	}
	SetCache(nil)

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 upstream fetches (initial + bypass), got %d", got)
	}
}

// TestCachedFetchTags_DisabledCache never calls Get/Set, so every lookup
// hits the fetcher.
func TestCachedFetchTags_DisabledCache(t *testing.T) {
	var calls int32
	origFetch := fetchTagsFunc
	fetchTagsFunc = func(_ *http.Client, _, _ string) ([]string, error) {
		atomic.AddInt32(&calls, 1)
		return []string{"v1"}, nil
	}
	defer func() { fetchTagsFunc = origFetch }()

	SetCache(cache.NewAt(t.TempDir(), time.Hour, false))
	defer SetCache(nil)

	for i := 0; i < 3; i++ {
		if _, err := cachedFetchTags("reg", "repo"); err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("disabled cache should not short-circuit; got %d fetches, want 3", got)
	}
}
