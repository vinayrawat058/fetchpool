package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommitSkipsDuplicateHash(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracker(dir)

	a := filepath.Join(dir, "a.bin")
	b := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(a, []byte("same-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("same-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash := "dummy-hash"
	if _, dup, err := tr.Commit("https://example.com/a", a, hash); err != nil || dup {
		t.Fatalf("first commit: dup=%v err=%v", dup, err)
	}

	existing, dup, err := tr.Commit("https://example.com/b", b, hash)
	if err != nil {
		t.Fatal(err)
	}
	if !dup || existing != a {
		t.Fatalf("expected duplicate of %s, got dup=%v existing=%q", a, dup, existing)
	}
}

func TestClaimURLDetectsDuplicate(t *testing.T) {
	dir := t.TempDir()
	tr := NewTracker(dir)
	path := filepath.Join(dir, "file.bin")

	if _, ok := tr.ClaimURL("https://example.com/a"); !ok {
		t.Fatal("first claim should succeed")
	}
	if _, dup, err := tr.Commit("https://example.com/a", path, "h1"); err != nil || dup {
		t.Fatalf("commit: dup=%v err=%v", dup, err)
	}

	existing, ok := tr.ClaimURL("https://example.com/a")
	if ok || existing != path {
		t.Fatalf("expected duplicate URL, ok=%v existing=%q", ok, existing)
	}
}
