package main

import (
	"context"
	"testing"
)

func TestStandaloneConsumerWalkthrough(t *testing.T) {
	if err := walkthrough(context.Background()); err != nil {
		t.Fatal(err)
	}
}
