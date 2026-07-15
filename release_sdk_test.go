package orag_test

import (
	"context"
	"testing"

	orag "github.com/shikanon/orag"
)

func TestPublicReleaseClientConformance(t *testing.T) {
	var client orag.ReleaseClient = (*orag.Client)(nil)
	if client == nil {
		t.Fatal("release client interface must be implemented by Client")
	}
}

func TestPublicReleaseClientListsMockEnvironments(t *testing.T) {
	client, err := orag.New(context.Background(), orag.MockConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	environments, err := client.ListEnvironments(context.Background(), orag.ListEnvironmentsRequest{ProjectID: "project_default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(environments) != 3 || environments[0].ProjectID == "" {
		t.Fatalf("environments = %#v", environments)
	}
}
