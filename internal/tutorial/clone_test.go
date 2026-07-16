package tutorial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/project"
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
	if _, claimed, err := repo.Acquire(context.Background(), subject.TenantID, first.ID, now.Add(time.Minute)); err != nil || !claimed {
		t.Fatalf("Acquire() claimed=%v err=%v", claimed, err)
	}
	if _, advanced, err := repo.Advance(context.Background(), subject.TenantID, first.ID, CloneStageCreateProject, CloneStageValidateManifest, CloneStatusQueued, now.Add(time.Minute)); err != nil || !advanced {
		t.Fatalf("Advance(create) advanced=%v err=%v", advanced, err)
	}
	if _, claimed, err := repo.Acquire(context.Background(), subject.TenantID, first.ID, now.Add(time.Minute)); err != nil || !claimed {
		t.Fatalf("Acquire(validate) claimed=%v err=%v", claimed, err)
	}
	if _, advanced, err := repo.Advance(context.Background(), subject.TenantID, first.ID, CloneStageValidateManifest, CloneStageDownloadPack, CloneStatusQueued, now.Add(time.Minute)); err != nil || !advanced {
		t.Fatalf("Advance(validate) advanced=%v err=%v", advanced, err)
	}
	if _, claimed, err := repo.Acquire(context.Background(), subject.TenantID, first.ID, now.Add(time.Minute)); err != nil || !claimed {
		t.Fatalf("Acquire(download) claimed=%v err=%v", claimed, err)
	}
	if _, failed, err := repo.Fail(context.Background(), subject.TenantID, first.ID, CloneStageDownloadPack, "object_checksum_mismatch", now.Add(time.Minute)); err != nil || !failed {
		t.Fatalf("Fail() failed=%v err=%v", failed, err)
	}
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

func TestCloneRunCreatesProjectCopiesVerifiedPackAndMarksExperimentInstalled(t *testing.T) {
	content := []byte("tutorial corpus")
	hash := sha256.Sum256(content)
	manifest := `{"template_id":"text-rag","version":"1.0.0","tier":"quick","license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},"objects":[{"path":"corpus/data.txt","sha256":"` + hex.EncodeToString(hash[:]) + `","bytes":15,"content_type":"text/plain"}],"runtime":{"baseline":{"profile":"realtime","top_k":5},"documents":[{"object_path":"corpus/data.txt","name":"教程语料"}],"dataset":{"name":"教程评测","items":[{"query":"教程语料是什么？","ground_truth":"tutorial corpus","split":"eval"}]}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packs/text-rag/1.0.0/quick/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(manifest))
		case "/packs/text-rag/1.0.0/quick/corpus/data.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	repo := NewMemoryCloneRepository()
	svc := NewCloneService(catalogForCloneTest(t), repo, func() time.Time { return now })
	reader, err := NewPublicPackReader(server.URL+"/packs", 1024, 1024, time.Second, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	outputRoot := t.TempDir()
	store, err := NewLocalPrivateStore(outputRoot, "tutorial-experiments")
	if err != nil {
		t.Fatal(err)
	}
	projects := newFakeCloneProjects()
	svc.ConfigureInstaller(projects, reader, store)
	svc.ConfigureRuntime(ResourceInitializer{KnowledgeBases: kb.NewMemoryStore(), Datasets: dataset.NewService(dataset.NewMemoryRepository())})

	job, _, err := svc.Start(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, CloneRequest{
		TemplateID: "text-rag", Version: "1.0.0", Tier: "quick", ProjectName: "Text lab", IdempotencyKey: "run_1", LicenseAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Run(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, job.ID); err != nil {
		t.Fatal(err)
	}
	completed, err := svc.GetJob(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, job.ID)
	if err != nil || completed.Status != CloneStatusCompleted || completed.Stage != CloneStagePackInstalled {
		t.Fatalf("completed job = %#v, %v", completed, err)
	}
	seenStages := map[CloneStage]bool{}
	for _, event := range completed.Events {
		seenStages[event.Stage] = true
	}
	for _, stage := range []CloneStage{CloneStageCreateProject, CloneStageValidateManifest, CloneStageDownloadPack, CloneStageVerifyPack, CloneStageWritePrivate, CloneStageCreateResources, CloneStagePackInstalled} {
		if !seenStages[stage] {
			t.Fatalf("events did not record %q: %#v", stage, completed.Events)
		}
	}
	if _, err := projects.Get(context.Background(), "tenant_a", job.ProjectID); err != nil {
		t.Fatalf("project is absent: %v", err)
	}
	experiment, err := svc.GetExperiment(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, job.ProjectID)
	if err != nil || experiment.PackStatus != PackStatusInstalled || experiment.RuntimeStatus != "ready" || experiment.KnowledgeBaseID == "" || experiment.DatasetID == "" {
		t.Fatalf("experiment = %#v, %v", experiment, err)
	}
	output := filepath.Join(outputRoot, "tutorial-experiments", "tenant_a", job.ProjectID, job.ID, hex.EncodeToString(hash[:]))
	stored, err := os.ReadFile(output)
	if err != nil || string(stored) != string(content) {
		t.Fatalf("stored Pack = %q, %v", stored, err)
	}
}

func TestPublicExperimentExposesDeclaredVariantsWithoutManifest(t *testing.T) {
	experiment := Experiment{
		ID: "texp_1", PackStatus: PackStatusInstalled, RuntimeStatus: "ready",
		PackManifest: Manifest{Runtime: &RuntimeManifest{Candidates: []RuntimeCandidate{
			{ID: TutorialP1StructuredJSONCandidateID, Chapter: TutorialP1DocumentParserChapter, ParserMethod: TutorialStructuredJSONParserMethod},
			{ID: TutorialP2RecursiveChunkCandidateID, Chapter: TutorialP2ChunkingChapter, ParserMethod: "basic", ChunkSizeTokens: TutorialP2ChunkSizeTokens, ChunkOverlapTokens: TutorialP2ChunkOverlapTokens},
			{ID: TutorialP3ContextualCandidateID, Chapter: TutorialP3ContextualChapter, ParserMethod: "basic", ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens, ContextualRetrieval: true},
			{ID: TutorialP4SparseCandidateID, Chapter: TutorialP4SparseChapter, ParserMethod: "basic", ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens, RetrievalStrategy: TutorialRetrievalStrategySparse, ReuseBaselineIndex: true},
		}}},
	}
	public := publicExperiment(experiment)
	if len(public.Variants) != 5 || public.Variants[0].ID != "baseline" || public.Variants[0].RetrievalStrategy != TutorialRetrievalStrategyHybrid || public.Variants[0].ContextualRetrieval || public.Variants[0].ChunkSizeTokens != TutorialBaselineChunkSizeTokens || public.Variants[1].ID != TutorialP1StructuredJSONCandidateID || public.Variants[2].ID != TutorialP2RecursiveChunkCandidateID || public.Variants[2].ChunkSizeTokens != TutorialP2ChunkSizeTokens || public.Variants[2].ChunkOverlapTokens != TutorialP2ChunkOverlapTokens || public.Variants[2].ContextualRetrieval || public.Variants[3].ID != TutorialP3ContextualCandidateID || !public.Variants[3].ContextualRetrieval || public.Variants[4].ID != TutorialP4SparseCandidateID || public.Variants[4].RetrievalStrategy != TutorialRetrievalStrategySparse || !public.Variants[4].ReuseBaselineIndex || !public.Variants[4].Available {
		t.Fatalf("variants=%#v", public.Variants)
	}
	if public.PackManifest.Runtime != nil || len(public.PackManifest.Objects) != 0 {
		t.Fatalf("public experiment exposes private manifest: %#v", public.PackManifest)
	}
}

func TestCloneServiceRecoveryRequeuesInterruptedJobs(t *testing.T) {
	repo := NewMemoryCloneRepository()
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	svc := NewCloneService(catalogForCloneTest(t), repo, func() time.Time { return now })
	job, _, err := svc.Start(context.Background(), Subject{TenantID: "tenant_a", ID: "user_a"}, CloneRequest{
		TemplateID: "text-rag", Version: "1.0.0", Tier: "quick", ProjectName: "Text lab", IdempotencyKey: "recover_1", LicenseAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := repo.Acquire(context.Background(), job.TenantID, job.ID, now); err != nil || !claimed {
		t.Fatalf("Acquire() = claimed=%v err=%v", claimed, err)
	}
	pending, err := svc.RecoverPending(context.Background())
	if err != nil || len(pending) != 1 || pending[0].Status != CloneStatusQueued {
		t.Fatalf("RecoverPending() = %#v, %v", pending, err)
	}
	if got := pending[0].Events[len(pending[0].Events)-1]; got.Outcome != "recovered" || got.Stage != CloneStageCreateProject {
		t.Fatalf("recovery event = %#v", got)
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

type fakeCloneProjects struct {
	mu       sync.Mutex
	projects map[string]project.Project
}

func newFakeCloneProjects() *fakeCloneProjects {
	return &fakeCloneProjects{projects: map[string]project.Project{}}
}

func (s *fakeCloneProjects) CreateWithID(_ context.Context, tenantID, projectID string, input project.CreateInput) (project.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[projectID]; exists {
		return project.Project{}, project.ErrConflict
	}
	item := project.Project{ID: projectID, TenantID: tenantID, Name: input.Name, Description: input.Description}
	s.projects[projectID] = item
	return item, nil
}

func (s *fakeCloneProjects) Get(_ context.Context, tenantID, projectID string) (project.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.projects[projectID]
	if !ok || item.TenantID != tenantID {
		return project.Project{}, project.ErrNotFound
	}
	return item, nil
}
