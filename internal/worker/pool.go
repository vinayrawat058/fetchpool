package worker

import (
	"sync"

	"concurrent-downloader/internal/downloader"
)

// Pool runs download jobs with a fixed number of workers.
type Pool struct {
	Workers int
	Client  *downloader.Client
}

// Run downloads all jobs concurrently and returns one Result per job.
func (p *Pool) Run(jobs []downloader.Job) []downloader.Result {
	workers := p.Workers
	if workers <= 0 {
		workers = 4
	}
	if workers > len(jobs) && len(jobs) > 0 {
		workers = len(jobs)
	}

	client := p.Client
	if client == nil {
		client = downloader.NewClient(0)
	}

	jobCh := make(chan downloader.Job)
	resultCh := make(chan downloader.Result, len(jobs))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				resultCh <- client.Download(job)
			}
		}()
	}

	go func() {
		for _, job := range jobs {
			jobCh <- job
		}
		close(jobCh)
		wg.Wait()
		close(resultCh)
	}()

	results := make([]downloader.Result, 0, len(jobs))
	for r := range resultCh {
		results = append(results, r)
	}
	return results
}
