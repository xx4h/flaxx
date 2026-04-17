package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetSetRoundTrip(t *testing.T) {
	c := NewAt(t.TempDir(), time.Hour, true)

	want := []string{"v1.0.0", "v1.1.0", "v2.0.0"}
	if err := c.Set("k1", want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got []string
	hit, err := c.Get("k1", &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !hit {
		t.Fatal("expected hit")
	}
	if len(got) != 3 || got[0] != "v1.0.0" || got[2] != "v2.0.0" {
		t.Errorf("payload mismatch: %v", got)
	}
}

func TestGetMissingKey(t *testing.T) {
	c := NewAt(t.TempDir(), time.Hour, true)
	var out []string
	hit, err := c.Get("nope", &out)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hit {
		t.Error("expected miss on missing key")
	}
}

func TestExpiredEntryIsMiss(t *testing.T) {
	dir := t.TempDir()
	c := NewAt(dir, time.Millisecond, true)

	if err := c.Set("k", []string{"x"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	var out []string
	hit, err := c.Get("k", &out)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hit {
		t.Error("expected miss on expired entry")
	}
}

func TestDisabledCacheIsNoop(t *testing.T) {
	dir := t.TempDir()
	c := NewAt(dir, time.Hour, false)

	if err := c.Set("k", []string{"x"}); err != nil {
		t.Fatalf("Set on disabled cache should be no-op, got %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("disabled Set wrote %d files, want 0", len(entries))
	}

	var out []string
	hit, _ := c.Get("k", &out)
	if hit {
		t.Error("disabled cache should never hit")
	}
}

func TestNilCacheIsSafe(t *testing.T) {
	var c *Cache
	if err := c.Set("k", 1); err != nil {
		t.Errorf("nil Set should not error: %v", err)
	}
	var out int
	hit, err := c.Get("k", &out)
	if err != nil || hit {
		t.Errorf("nil Get returned hit=%v err=%v", hit, err)
	}
}

func TestWithBypassReadStillWrites(t *testing.T) {
	dir := t.TempDir()
	c := NewAt(dir, time.Hour, true)
	if err := c.Set("k", []string{"v1"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	bc := c.WithBypassRead()
	var out []string
	hit, _ := bc.Get("k", &out)
	if hit {
		t.Error("bypass Get should miss even when file exists")
	}

	// Write-through still works
	if err := bc.Set("k", []string{"v2"}); err != nil {
		t.Fatalf("bypass Set: %v", err)
	}
	// Non-bypass Get should now see the refreshed value
	hit, _ = c.Get("k", &out)
	if !hit || out[0] != "v2" {
		t.Errorf("expected refreshed value v2, got hit=%v out=%v", hit, out)
	}
}

func TestCorruptedFileTreatedAsMiss(t *testing.T) {
	dir := t.TempDir()
	c := NewAt(dir, time.Hour, true)

	path := filepath.Join(dir, Key("helm", "x", "y")+".json")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out []string
	hit, err := c.Get(Key("helm", "x", "y"), &out)
	if err != nil {
		t.Fatalf("Get on corrupted file returned error: %v", err)
	}
	if hit {
		t.Error("corrupted file should be treated as miss")
	}
}

func TestAtomicRenameLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	c := NewAt(dir, time.Hour, true)

	for i := 0; i < 5; i++ {
		if err := c.Set("k", []string{"v"}); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name()[0] == '.' {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestKeyIsStableAndDistinct(t *testing.T) {
	a := Key("helm", "https://charts.example.com", "mychart")
	b := Key("helm", "https://charts.example.com", "mychart")
	c := Key("oci", "https://charts.example.com", "mychart")

	if a != b {
		t.Error("Key should be stable for identical inputs")
	}
	if a == c {
		t.Error("different prefixes should produce different keys")
	}
	if len(a) != 64 {
		t.Errorf("expected 64-char sha256 hex, got %d", len(a))
	}
}
