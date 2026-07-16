package tutorial

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVideoImportPersistsVerifiedPrivateSourceAndSegments(t *testing.T) {
	b := []byte("authorized video")
	h := sha256.Sum256(b)
	repo := NewMemoryCloneRepository()
	store, err := NewLocalPrivateStore(t.TempDir(), "private")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseVideoProtocol([]byte(validVideoProtocol), videoProtocolTemplate(t), videoProtocolPack(t))
	if err != nil {
		t.Fatal(err)
	}
	exp := Experiment{ID: "e", TenantID: "t", ProjectID: "p", CloneJobID: "j", TemplateID: "video-rag", PackStatus: PackStatusInstalled, PackManifest: Manifest{VideoProtocol: &p}}
	if err := repo.EnsureExperiment(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
	source := VideoSource{Alias: "clip", SHA256: hex.EncodeToString(h[:]), Bytes: int64(len(b)), ContentType: "video/mp4", DurationMS: 10000}
	_, segments, err := NewVideoImportService(repo, store, t.TempDir()).Import(context.Background(), Subject{TenantID: "t"}, "p", source, bytes.NewReader(b))
	if err != nil || len(segments) != 1 || segments[0].EvidenceID != "clip@0-10000" {
		t.Fatalf("segments=%#v err=%v", segments, err)
	}
	got, found, err := repo.GetExperiment(context.Background(), "t", "p")
	if err != nil || !found || got.PackManifest.VideoSource == nil || len(got.PackManifest.TemporalSegments) != 1 || len(got.PackManifest.TemporalAssets) != 1 || got.RuntimeStatus != "temporal_index_pending_evaluation" {
		t.Fatalf("experiment=%#v err=%v", got, err)
	}
	privateIndex := PrivateObject{
		TenantID:  "t",
		ProjectID: "p",
		JobID:     "j",
		Object:    VerifiedObject{PackObject: got.PackManifest.TemporalAssets[0]},
	}
	index, err := store.ReadVerified(context.Background(), privateIndex)
	if err != nil || string(index) != "evidence=clip@0-10000\nstart_ms=0\nend_ms=10000\nsubtitle=\n" || bytes.Contains(index, []byte("authorized video")) {
		t.Fatalf("temporal index=%q err=%v", index, err)
	}
}
