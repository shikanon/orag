package loader

import (
	"context"
	"os"
	"path/filepath"
)

type Local struct{}

func (Local) Load(_ context.Context, source string) (Document, error) {
	content, err := os.ReadFile(source)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: filepath.Base(source), SourceURI: "file://" + source, Content: content}, nil
}
