package loader

import (
	"context"
	"fmt"
)

type TOS struct{}

func (TOS) Load(_ context.Context, source string) (Document, error) {
	return Document{}, fmt.Errorf("tos loader requires configured 火山 TOS client: %s", source)
}
