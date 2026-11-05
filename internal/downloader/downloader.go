package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Job is a single URL download request.
type Job struct {
	URL      string
	OutDir   string
	FileName string // optional override; empty = derive from URL
}

// Result holds the outcome of one download.
type Result struct {
	URL        string
	FilePath   string
	Bytes      int64
	Hash       string
	Skipped    bool
	SkipReason string
	Err        error
	Duration   time.Duration
}

// Client downloads files over HTTP(S).
type Client struct {
	HTTPClient *http.Client
	Tracker    *Tracker
	OnProgress ProgressFunc // optional live progress callback
	MaxBytes   int64        // 0 = unlimited; reject files larger than this
}

// NewClient returns a Client with a sensible default timeout.
func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

// Download fetches job.URL and writes it under job.OutDir.
// Duplicate URLs and identical content hashes are skipped when Tracker is set.
func (c *Client) Download(job Job) Result {
	start := time.Now()
	res := Result{URL: job.URL}

	if job.URL == "" {
		res.Err = fmt.Errorf("empty URL")
		res.Duration = time.Since(start)
		return res
	}

	outDir := job.OutDir
	if outDir == "" {
		outDir = "downloads"
	}

	if c.Tracker != nil {
		if existing, ok := c.Tracker.ClaimURL(job.URL); !ok {
			res.Skipped = true
			res.SkipReason = "duplicate URL"
			res.FilePath = existing
			res.Duration = time.Since(start)
			return res
		}
	}

	req, err := http.NewRequest(http.MethodGet, job.URL, nil)
	if err != nil {
		c.releaseURL(job.URL)
		res.Err = fmt.Errorf("build request: %w", err)
		res.Duration = time.Since(start)
		return res
	}
	req.Header.Set("User-Agent", "concurrent-downloader/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		c.releaseURL(job.URL)
		res.Err = fmt.Errorf("request failed: %w", err)
		res.Duration = time.Since(start)
		return res
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.releaseURL(job.URL)
		res.Err = fmt.Errorf("bad status %d", resp.StatusCode)
		res.Duration = time.Since(start)
		return res
	}

	var total int64
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	if c.MaxBytes > 0 && total > c.MaxBytes {
		c.releaseURL(job.URL)
		res.Skipped = true
		res.SkipReason = fmt.Sprintf("exceeds max size (%s > %s)", FormatBytes(total), FormatBytes(c.MaxBytes))
		res.Duration = time.Since(start)
		return res
	}

	name := job.FileName
	if name == "" {
		name = fileNameFromURL(job.URL)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		c.releaseURL(job.URL)
		res.Err = fmt.Errorf("create output dir: %w", err)
		res.Duration = time.Since(start)
		return res
	}

	filePath := uniquePath(filepath.Join(outDir, name))
	f, err := os.Create(filePath)
	if err != nil {
		c.releaseURL(job.URL)
		res.Err = fmt.Errorf("create file: %w", err)
		res.Duration = time.Since(start)
		return res
	}

	hasher := sha256.New()
	base := io.MultiWriter(f, hasher)
	var writer io.Writer = base
	var pw *progressWriter
	if c.OnProgress != nil {
		pw = newProgressWriter(base, job.URL, name, total, c.OnProgress)
		writer = pw
	}

	src := io.Reader(resp.Body)
	if c.MaxBytes > 0 {
		// Read at most MaxBytes+1 so we can detect oversized bodies without Content-Length.
		src = io.LimitReader(resp.Body, c.MaxBytes+1)
	}

	n, err := io.Copy(writer, src)
	if pw != nil {
		pw.finish()
	}
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(filePath)
		c.releaseURL(job.URL)
		res.Err = fmt.Errorf("write file: %w", err)
		res.Duration = time.Since(start)
		return res
	}
	if c.MaxBytes > 0 && n > c.MaxBytes {
		_ = os.Remove(filePath)
		c.releaseURL(job.URL)
		res.Skipped = true
		res.SkipReason = fmt.Sprintf("exceeds max size (>%s)", FormatBytes(c.MaxBytes))
		res.Bytes = 0
		res.Duration = time.Since(start)
		return res
	}
	if closeErr != nil {
		_ = os.Remove(filePath)
		c.releaseURL(job.URL)
		res.Err = fmt.Errorf("close file: %w", closeErr)
		res.Duration = time.Since(start)
		return res
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	res.Hash = sum
	res.Bytes = n

	if c.Tracker != nil {
		existing, dup, err := c.Tracker.Commit(job.URL, filePath, sum)
		if err != nil {
			res.Err = fmt.Errorf("saved file but failed to update index: %w", err)
			res.FilePath = filePath
			res.Duration = time.Since(start)
			return res
		}
		if dup {
			_ = os.Remove(filePath)
			res.Skipped = true
			res.SkipReason = "duplicate hash"
			res.FilePath = existing
			res.Bytes = 0
			res.Duration = time.Since(start)
			return res
		}
	}

	res.FilePath = filePath
	res.Duration = time.Since(start)
	return res
}

func (c *Client) releaseURL(u string) {
	if c.Tracker != nil {
		c.Tracker.ReleaseURL(u)
	}
}

func fileNameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" || u.Path == "/" {
		return "download"
	}
	base := path.Base(u.Path)
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == "/" {
		return "download"
	}
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	return base
}

// uniquePath appends -1, -2, ... if the path already exists.
func uniquePath(p string) string {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", stem, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
