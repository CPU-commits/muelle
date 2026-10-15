package cron

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestJobs(t *testing.T) {
	boom, bang := errors.New("boom"), errors.New("bang")
	proxy, certs := 0, 0
	jobs := Jobs(func(ctx context.Context) error { proxy++; return boom }, func(ctx context.Context) error { certs++; return bang })
	if len(jobs) != 2 || jobs[0].Name != "proxy" || jobs[0].Every != 5*time.Minute ||
		jobs[1].Name != "certificados" || jobs[1].Every != time.Hour {
		t.Fatalf("jobs: %+v", jobs)
	}
	if err := jobs[0].Run(context.Background()); !errors.Is(err, boom) || proxy != 1 || certs != 0 {
		t.Fatalf("proxy: %v (%d, %d llamadas)", err, proxy, certs)
	}
	if err := jobs[1].Run(context.Background()); !errors.Is(err, bang) || certs != 1 {
		t.Fatalf("certificados: %v (%d llamadas)", err, certs)
	}
}
