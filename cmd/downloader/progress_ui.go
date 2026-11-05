package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"concurrent-downloader/internal/downloader"
)

// progressUI shows a live multi-download status line on stderr.
type progressUI struct {
	mu     sync.Mutex
	active map[string]downloader.Progress
	stop   chan struct{}
	done   chan struct{}
}

func newProgressUI() *progressUI {
	return &progressUI{
		active: make(map[string]downloader.Progress),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (ui *progressUI) Handler() downloader.ProgressFunc {
	return func(p downloader.Progress) {
		ui.mu.Lock()
		defer ui.mu.Unlock()
		if p.Done {
			delete(ui.active, p.URL)
			return
		}
		ui.active[p.URL] = p
	}
}

func (ui *progressUI) Start() {
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		defer close(ui.done)
		for {
			select {
			case <-ui.stop:
				ui.clearLine()
				return
			case <-ticker.C:
				ui.render()
			}
		}
	}()
}

func (ui *progressUI) Stop() {
	close(ui.stop)
	<-ui.done
}

func (ui *progressUI) render() {
	ui.mu.Lock()
	items := make([]downloader.Progress, 0, len(ui.active))
	for _, p := range ui.active {
		items = append(items, p)
	}
	ui.mu.Unlock()

	if len(items) == 0 {
		return
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	parts := make([]string, 0, len(items))
	for _, p := range items {
		parts = append(parts, formatProgressItem(p))
	}

	line := fmt.Sprintf("  ↓ %d active | %s", len(items), strings.Join(parts, " · "))
	if len(line) > 120 {
		line = line[:117] + "..."
	}
	fmt.Fprintf(os.Stderr, "\r%-120s", line)
}

func (ui *progressUI) clearLine() {
	fmt.Fprintf(os.Stderr, "\r%-120s\r", "")
}

func formatProgressItem(p downloader.Progress) string {
	name := p.Name
	if len(name) > 18 {
		name = name[:15] + "..."
	}
	read := downloader.FormatBytes(p.BytesRead)
	speed := downloader.FormatSpeed(p.SpeedBPS)
	if p.Total > 0 {
		pct := float64(p.BytesRead) * 100 / float64(p.Total)
		return fmt.Sprintf("%s %.0f%% %s/%s %s",
			name, pct, read, downloader.FormatBytes(p.Total), speed)
	}
	return fmt.Sprintf("%s %s %s", name, read, speed)
}
