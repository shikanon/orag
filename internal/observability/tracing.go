package observability

import "context"

type Span struct {
	Name string
}

func StartSpan(ctx context.Context, name string) (context.Context, Span) {
	return ctx, Span{Name: name}
}

func (Span) End(error) {}
