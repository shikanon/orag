package contract

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestEinoJinjaDisablesFilesystemFilters(t *testing.T) {
	directory := t.TempDir()
	secret := "orag-jinja-filesystem-canary"
	secretPath := filepath.Join(directory, "secret.txt")
	if err := os.WriteFile(secretPath, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		template string
		key      string
		value    string
		want     string
	}{
		{name: "file", template: `{{ path | file }}`, key: "path", value: secretPath, want: "keyword[file] has been disabled"},
		{name: "fileset", template: `{{ pattern | fileset }}`, key: "pattern", value: filepath.Join(directory, "*.txt"), want: "keyword[fileset] has been disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := schema.UserMessage(tt.template).Format(
				context.Background(),
				map[string]any{tt.key: tt.value},
				schema.Jinja2,
			)
			if err == nil {
				for _, message := range messages {
					if strings.Contains(message.Content, secret) || strings.Contains(message.Content, secretPath) {
						t.Fatalf("Jinja %s filter exposed filesystem content: %q", tt.name, message.Content)
					}
				}
				t.Fatalf("Jinja %s filter unexpectedly succeeded: %#v", tt.name, messages)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Jinja %s error = %v, want %q", tt.name, err, tt.want)
			}
		})
	}
}
