package loader

import "context"

type Document struct {
	Name      string
	SourceURI string
	Content   []byte
}

type Loader interface {
	Load(ctx context.Context, source string) (Document, error)
}
