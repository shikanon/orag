package contract

import (
	"strings"
	"testing"
)

func TestDockerComposeAPIUsesContainerNetworkDefaults(t *testing.T) {
	compose := readRepoFile(t, "deployments/docker-compose.yml")

	for _, want := range []string{
		`DATABASE_URL: ${DOCKER_DATABASE_URL:-postgres://orag:orag@postgres:5432/orag?sslmode=disable}`,
		`QDRANT_HOST: ${DOCKER_QDRANT_HOST:-qdrant}`,
		`QDRANT_GRPC_PORT: ${DOCKER_QDRANT_GRPC_PORT:-6334}`,
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("docker-compose.yml orag-api service must override host-local .env with %q", want)
		}
	}

	for _, bad := range []string{
		`DATABASE_URL: ${DATABASE_URL:-postgres://orag:orag@localhost:5432/orag?sslmode=disable}`,
		`QDRANT_HOST: ${QDRANT_HOST:-localhost}`,
	} {
		if strings.Contains(compose, bad) {
			t.Fatalf("docker-compose.yml must not default container dependencies to host-local value %q", bad)
		}
	}
}
