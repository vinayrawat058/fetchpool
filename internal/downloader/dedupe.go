package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const indexFileName = ".fetchpool-index.json"

// Tracker remembers URLs and content hashes so duplicates can be skipped.
type Tracker struct {
	mu     sync.Mutex
	urls   map[string]string // url -> file path
	hashes map[string]string // sha256 hex -> file path
	outDir string
}

type indexFile struct {
	URLs   map[string]string `json:"urls"`
	Hashes map[string]string `json:"hashes"`
}

// NewTracker creates an empty tracker for outDir.
func NewTracker(outDir string) *Tracker {
	if outDir == "" {
		outDir = "downloads"
	}
	return &Tracker{
		urls:   make(map[string]string),
		hashes: make(map[string]string),
		outDir: outDir,
	}
}

// LoadTracker loads the on-disk index for outDir, drops missing files,
// and indexes any other regular files already in the folder by content hash.
func LoadTracker(outDir string) (*Tracker, error) {
	t := NewTracker(outDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	if err := t.loadIndex(); err != nil {
		return nil, err
	}
	if err := t.scanUnindexedFiles(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Tracker) indexPath() string {
	return filepath.Join(t.outDir, indexFileName)
}

func (t *Tracker) loadIndex() error {
	data, err := os.ReadFile(t.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read index: %w", err)
	}

	var idx indexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		return fmt.Errorf("parse index: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for u, p := range idx.URLs {
		if fileExists(p) {
			t.urls[u] = p
		}
	}
	for h, p := range idx.Hashes {
		if fileExists(p) {
			t.hashes[h] = p
		}
	}
	return nil
}

func (t *Tracker) scanUnindexedFiles() error {
	entries, err := os.ReadDir(t.outDir)
	if err != nil {
		return fmt.Errorf("scan output dir: %w", err)
	}

	knownPaths := make(map[string]struct{})
	t.mu.Lock()
	for _, p := range t.hashes {
		knownPaths[filepath.Clean(p)] = struct{}{}
	}
	t.mu.Unlock()

	for _, e := range entries {
		if e.IsDir() || e.Name() == indexFileName || e.Name() == ".gitkeep" {
			continue
		}
		path := filepath.Join(t.outDir, e.Name())
		if _, ok := knownPaths[filepath.Clean(path)]; ok {
			continue
		}
		sum, err := hashFile(path)
		if err != nil {
			continue
		}
		t.mu.Lock()
		if existing, ok := t.hashes[sum]; !ok {
			t.hashes[sum] = path
		} else if existing != path {
			// Same content already indexed under another path; keep the first.
		}
		t.mu.Unlock()
	}
	return nil
}

func (t *Tracker) saveLocked() error {
	idx := indexFile{
		URLs:   make(map[string]string, len(t.urls)),
		Hashes: make(map[string]string, len(t.hashes)),
	}
	for u, p := range t.urls {
		idx.URLs[u] = p
	}
	for h, p := range t.hashes {
		idx.Hashes[h] = p
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(t.indexPath(), data, 0o644)
}

// ClaimURL reserves url for download. If it was already seen, ok is false
// and existing is the path of the prior file.
func (t *Tracker) ClaimURL(url string) (existing string, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, found := t.urls[url]; found {
		return p, false
	}
	// Placeholder so concurrent workers with the same URL skip.
	t.urls[url] = ""
	return "", true
}

// ReleaseURL clears a failed claim so the URL can be retried later.
func (t *Tracker) ReleaseURL(url string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.urls[url]; ok && p == "" {
		delete(t.urls, url)
	}
}

// Commit records a successful download. If hash is already known, it binds
// url to the existing path, returns that path, and sets duplicate=true.
func (t *Tracker) Commit(url, path, hash string) (existing string, duplicate bool, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.hashes[hash]; ok {
		t.urls[url] = p
		if err := t.saveLocked(); err != nil {
			return p, true, err
		}
		return p, true, nil
	}
	t.urls[url] = path
	t.hashes[hash] = path
	if err := t.saveLocked(); err != nil {
		return "", false, err
	}
	return "", false, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
