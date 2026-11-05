package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"concurrent-downloader/internal/agent"
	"concurrent-downloader/internal/downloader"
	"concurrent-downloader/internal/worker"
)

func main() {
	var (
		urlFile     = flag.String("file", "", "path to a text file with one URL per line")
		outDir      = flag.String("out", "downloads", "directory to save files into")
		workers     = flag.Int("workers", 4, "max concurrent downloads")
		timeout     = flag.Duration("timeout", 60*time.Second, "per-request HTTP timeout")
		urlsFlag    = flag.String("urls", "", "comma-separated list of URLs")
		dedupe      = flag.Bool("dedupe", true, "skip duplicate URLs and identical file hashes")
		progress    = flag.Bool("progress", true, "show live download progress")
		agentPrompt = flag.String("agent", "", "AI agent prompt: propose download URLs from natural language")
		agentDry    = flag.Bool("agent-dry-run", false, "only show URLs proposed by the agent; do not download")
		agentLimit  = flag.Int("agent-limit", 5, "max URLs the agent may propose")
	)
	flag.Parse()

	urls, dupesInList, err := collectURLs(*urlsFlag, *urlFile, flag.Args())
	if err != nil {
		fail(err)
	}

	if strings.TrimSpace(*agentPrompt) != "" {
		cfg, err := agent.FromEnv()
		if err != nil {
			fail(fmt.Errorf("%w\nSet OPENAI_API_KEY (optional: OPENAI_BASE_URL, OPENAI_MODEL)", err))
		}
		cfg.Limit = *agentLimit
		ai := agent.New(cfg)

		fmt.Printf("Agent planning downloads for: %q\n", *agentPrompt)
		proposed, summary, err := ai.ProposeURLs(*agentPrompt)
		if err != nil {
			fail(err)
		}
		if summary != "" {
			fmt.Printf("Agent: %s\n", summary)
		}
		fmt.Printf("Proposed %d URL(s):\n", len(proposed))
		for i, u := range proposed {
			fmt.Printf("  %d. %s\n", i+1, u)
		}
		if *agentDry {
			fmt.Println("\nDry run only — nothing downloaded.")
			return
		}
		// Merge agent URLs with any URLs also passed on the CLI.
		for _, u := range proposed {
			urls = appendUnique(urls, u)
		}
	}

	if len(urls) == 0 {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] <url> [url...]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "       or pass -urls / -file / -agent")
		flag.PrintDefaults()
		os.Exit(2)
	}

	jobs := make([]downloader.Job, 0, len(urls))
	for _, u := range urls {
		jobs = append(jobs, downloader.Job{URL: u, OutDir: *outDir})
	}

	client := downloader.NewClient(*timeout)
	if *dedupe {
		tracker, err := downloader.LoadTracker(*outDir)
		if err != nil {
			fail(err)
		}
		client.Tracker = tracker
	}

	var ui *progressUI
	if *progress {
		ui = newProgressUI()
		client.OnProgress = ui.Handler()
		ui.Start()
	}

	fmt.Printf("Downloading %d file(s) with %d worker(s) → %s\n", len(jobs), *workers, *outDir)
	if dupesInList > 0 {
		fmt.Printf("Ignored %d duplicate URL(s) in the input list\n", dupesInList)
	}

	pool := &worker.Pool{
		Workers: *workers,
		Client:  client,
	}

	start := time.Now()
	results := pool.Run(jobs)
	if ui != nil {
		ui.Stop()
	}

	var ok, failed, skipped int
	var totalBytes int64
	for _, r := range results {
		if r.Err != nil {
			failed++
			fmt.Printf("FAIL  %s\n      %v\n", r.URL, r.Err)
			continue
		}
		if r.Skipped {
			skipped++
			fmt.Printf("SKIP  %s\n      %s → %s\n", r.URL, r.SkipReason, r.FilePath)
			continue
		}
		ok++
		totalBytes += r.Bytes
		hashNote := ""
		if r.Hash != "" {
			hashNote = fmt.Sprintf(", sha256=%s", shortHash(r.Hash))
		}
		fmt.Printf("OK    %s\n      → %s (%s in %s%s)\n",
			r.URL, r.FilePath, downloader.FormatBytes(r.Bytes), r.Duration.Round(time.Millisecond), hashNote)
	}

	fmt.Printf("\nDone in %s — %d ok, %d skipped, %d failed, %s total\n",
		time.Since(start).Round(time.Millisecond), ok, skipped, failed, downloader.FormatBytes(totalBytes))

	if failed > 0 {
		os.Exit(1)
	}
}

func appendUnique(list []string, u string) []string {
	for _, existing := range list {
		if existing == u {
			return list
		}
	}
	return append(list, u)
}

func collectURLs(commaList, filePath string, args []string) ([]string, int, error) {
	seen := make(map[string]struct{})
	var out []string
	var dupes int

	add := func(raw string) {
		u := strings.TrimSpace(raw)
		if u == "" || strings.HasPrefix(u, "#") {
			return
		}
		if _, ok := seen[u]; ok {
			dupes++
			return
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}

	for _, u := range args {
		add(u)
	}
	if commaList != "" {
		for _, u := range strings.Split(commaList, ",") {
			add(u)
		}
	}
	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, 0, fmt.Errorf("open URL file: %w", err)
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			add(sc.Text())
		}
		if err := sc.Err(); err != nil {
			return nil, 0, fmt.Errorf("read URL file: %w", err)
		}
	}
	return out, dupes, nil
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
