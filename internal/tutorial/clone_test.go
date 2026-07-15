package tutorial

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCloneStartIsIdempotentAndRetryResumesCheckpoint(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	repo := NewMemoryCloneRepository()
	svc := NewCloneService(catalogForCloneTest(t), repo, func() time.Time { return now })
	input := CloneRequest{TemplateID: "text-rag", Version: "1.0.0", Tier: "quick", ProjectName: "Text lab", IdempotencyKey: "req_1", LicenseAccepted: true}
	subject := Subject{TenantID: "tenant_a", ID: "user_a"}
	first, replayed, err := svc.Start(context.Background(), subject, input)
	if err != nil || replayed {
		t.Fatalf("first=%#v replayed=%v err=%v", first, replayed, err)
	}
	again, replayed, err := svc.Start(context.Background(), subject, input)
	if err != nil || !replayed || again.ID != first.ID || again.ProjectID != first.ProjectID || len(repo.Jobs()) != 1 {
		t.Fatalf("again=%#v replayed=%v jobs=%d err=%v", again, replayed, len(repo.Jobs()), err)
	}
	repo.Fail(first.ID, CloneStageVerifyPack, "object_checksum_mismatch", now.Add(time.Minute))
	retried, err := svc.Retry(context.Background(), subject, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Stage != CloneStageDownloadPack || retried.Status != CloneStatusQueued || retried.Attempt != 2 || retried.LastErrorCode != "" {
		t.Fatalf("Retry() = %#v", retried)
	}
}

func TestCloneJobTenantIsolationAndValidation(t *testing.T) {
	repo := NewMemoryCloneRepository()
	svc := NewCloneService(catalogForCloneTest(t), repo, time.Now)
	input := CloneRequest{TemplateID: "text-rag", Version: "1.0.0", Tier: "quick", ProjectName: "Text lab", IdempotencyKey: "req_1", LicenseAccepted: true}
	job, _, err := svc.Start(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetJob(context.Background(), Subject{TenantID: "tenant_b", ID: "user_b"}, job.ID); !errors.Is(err, ErrCloneJobNotFound) {
		t.Fatalf("foreign GetJob error = %v", err)
	}
	if _, _, err := svc.Start(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, CloneRequest{TemplateID: "text-rag", Version: "1.0.0", Tier: "quick", ProjectName: "Text lab", IdempotencyKey: "req_2"}); !errors.Is(err, ErrCloneLicenseRequired) {
		t.Fatalf("license error = %v", err)
	}
	if _, _, err := svc.Start(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, CloneRequest{TemplateID: "text-rag", Version: "1.0.0", Tier: "unknown", ProjectName: "Text lab", IdempotencyKey: "req_3", LicenseAccepted: true}); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("tier error = %v", err)
	}
}

func TestCloneStartHasSingleWinnerUnderConcurrency(t *testing.T) {
	repo := NewMemoryCloneRepository()
	svc := NewCloneService(catalogForCloneTest(t), repo, time.Now)
	input := CloneRequest{TemplateID: "text-rag", Version: "1.0.0", Tier: "quick", ProjectName: "Text lab", IdempotencyKey: "same", LicenseAccepted: true}
	subject := Subject{TenantID: "tenant_a", ID: "user_a"}
	start := make(chan struct{})
	ids := make(chan string, 16)
	errs := make(chan error, 16)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			job, _, err := svc.Start(context.Background(), subject, input)
			ids <- job.ID
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errs)
	var winner string
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for jobID := range ids {
		if winner == "" {
			winner = jobID
		}
		if jobID != winner {
			t.Fatalf("job IDs differ: %q and %q", winner, jobID)
		}
	}
	if len(repo.Jobs()) != 1 {
		t.Fatalf("stored jobs = %d, want 1", len(repo.Jobs()))
	}
}

func catalogForCloneTest(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
