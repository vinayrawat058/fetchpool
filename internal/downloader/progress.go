package downloader

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Progress is a snapshot of one download's transfer state.
type Progress struct {
	URL       string
	Name      string
	BytesRead int64
	Total     int64 // 0 if unknown (no Content-Length)
	SpeedBPS  int64 // bytes per second (instantaneous / recent window)
	Done      bool
}

// ProgressFunc is called with live download updates (may be from many goroutines).
type ProgressFunc func(Progress)

// progressWriter wraps a writer, hashing/counting bytes and emitting throttled progress.
type progressWriter struct {
	w          io.Writer
	url        string
	name       string
	total      int64
	read       int64
	start      time.Time
	lastEmit   time.Time
	lastBytes  int64
	onProgress ProgressFunc
	mu         sync.Mutex
}

func newProgressWriter(w io.Writer, url, name string, total int64, on ProgressFunc) *progressWriter {
	now := time.Now()
	pw := &progressWriter{
		w:          w,
		url:        url,
		name:       name,
		total:      total,
		start:      now,
		lastEmit:   now,
		onProgress: on,
	}
	pw.emit(false)
	return pw
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	if n > 0 {
		p.mu.Lock()
		p.read += int64(n)
		shouldEmit := time.Since(p.lastEmit) >= 200*time.Millisecond
		p.mu.Unlock()
		if shouldEmit {
			p.emit(false)
		}
	}
	return n, err
}

func (p *progressWriter) finish() {
	p.emit(true)
}

func (p *progressWriter) emit(done bool) {
	if p.onProgress == nil {
		return
	}
	p.mu.Lock()
	read := p.read
	elapsed := time.Since(p.lastEmit).Seconds()
	delta := read - p.lastBytes
	p.lastBytes = read
	p.lastEmit = time.Now()
	totalElapsed := time.Since(p.start).Seconds()
	p.mu.Unlock()

	var speed int64
	if done && totalElapsed > 0 {
		speed = int64(float64(read) / totalElapsed)
	} else if elapsed > 0 {
		speed = int64(float64(delta) / elapsed)
	}

	p.onProgress(Progress{
		URL:       p.url,
		Name:      p.name,
		BytesRead: read,
		Total:     p.total,
		SpeedBPS:  speed,
		Done:      done,
	})
}

// FormatBytes formats a byte count for display (e.g. "1.2 MB").
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// FormatSpeed formats bytes/sec for display.
func FormatSpeed(bps int64) string {
	if bps <= 0 {
		return "—"
	}
	return FormatBytes(bps) + "/s"
}
