package cron

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestJobs(t *testing.T) {
	boom := errors.New("boom")
	called := 0
	jobs := Jobs(func(ctx context.Context) error { called++; return boom })
	if len(jobs) != 1 || jobs[0].Name != "proxy" || jobs[0].Every != 5*time.Minute {
		t.Fatalf("jobs: %+v", jobs)
	}
	if err := jobs[0].Run(context.Background()); !errors.Is(err, boom) || called != 1 {
		t.Fatalf("Run: %v (%d llamadas)", err, called)
	}
}
