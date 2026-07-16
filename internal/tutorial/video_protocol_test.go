package tutorial

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const validVideoProtocol = `{
  "template_id":"video-rag",
  "version":"1.0.0",
  "tier":"quick",
  "benchmark":{"id":"Video-MME","source_url":"https://video-mme.github.io/home_page.html","import_mode":"owner_authorized_private_import"},
  "sampling":{"segment_milliseconds":10000,"max_segments":180},
  "runtime":{"profile":"temporal_page","top_k":5,"extractor_version":"temporal-v1"}
}`

func TestParseVideoProtocolAcceptsPrivateImportContract(t *testing.T) {
	protocol, err := ParseVideoProtocol([]byte(validVideoProtocol), videoProtocolTemplate(t), videoProtocolPack(t))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := protocol.EvidenceID("clip-a", 0, 10_000); err != nil || got != "clip-a@0-10000" {
		t.Fatalf("evidence=%q err=%v", got, err)
	}
}

func TestParseVideoProtocolRejectsPublicMediaAndRuntimeDrift(t *testing.T) {
	for name, raw := range map[string]string{
		"media_url":     validVideoProtocol[:len(validVideoProtocol)-1] + `,"media_url":"https://example.test/video.mp4"}`,
		"wrong_profile": `{"template_id":"video-rag","version":"1.0.0","tier":"quick","benchmark":{"id":"Video-MME","source_url":"https://video-mme.github.io/home_page.html","import_mode":"owner_authorized_private_import"},"sampling":{"segment_milliseconds":10000,"max_segments":180},"runtime":{"profile":"realtime","top_k":5,"extractor_version":"temporal-v1"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseVideoProtocol([]byte(raw), videoProtocolTemplate(t), videoProtocolPack(t)); !errors.Is(err, ErrVideoProtocolInvalid) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestPublishedVideoProtocolsMatchCatalog(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	template, err := catalog.Get("video-rag", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, tier := range []string{"quick", "benchmark"} {
		pack, ok := templatePack(template, tier)
		if !ok {
			t.Fatalf("%s artifact missing", tier)
		}
		raw, err := os.ReadFile(filepath.Join("..", "..", "tutorial-protocols", "video-rag", "1.0.0", tier, "protocol.json"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseVideoProtocol(raw, template, pack); err != nil {
			t.Fatalf("%s protocol: %v", tier, err)
		}
	}
}

func videoProtocolTemplate(t *testing.T) Template {
	t.Helper()
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	template, err := catalog.Get("video-rag", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	return template
}

func videoProtocolPack(t *testing.T) PackRef {
	t.Helper()
	pack, ok := templatePack(videoProtocolTemplate(t), "quick")
	if !ok {
		t.Fatal("quick protocol missing")
	}
	return pack
}
