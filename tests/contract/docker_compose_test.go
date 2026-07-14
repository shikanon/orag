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

func TestDockerComposeIncludesReleaseWalkthroughTopology(t *testing.T) {
	compose := readRepoFile(t, "deployments/docker-compose.yml")
	for _, want := range []string{
		"  migrate:",
		"condition: service_completed_successfully",
		"  orag-console:",
		"  demo:",
		"profiles: [\"demo\"]",
		"ALLOW_DETERMINISTIC_MOCK: \"true\"",
		"ORAG_DEMO_SUMMARY: /demo/walkthrough.json",
	} {
		if !strings.Contains(compose, want) {
			t.Errorf("docker-compose.yml missing %q", want)
		}
	}
}

func TestReleaseContainerDefinitionsExist(t *testing.T) {
	api := readRepoFile(t, "deployments/Dockerfile")
	for _, want := range []string{"AS api", "oragctl", "orag-demo", "pkg/buildinfo.Version"} {
		if !strings.Contains(api, want) {
			t.Errorf("API Dockerfile missing %q", want)
		}
	}
	console := readRepoFile(t, "deployments/console.Dockerfile")
	for _, want := range []string{"--platform=$BUILDPLATFORM", "npm ci", "npm run build", "nginx"} {
		if !strings.Contains(console, want) {
			t.Errorf("Console Dockerfile missing %q", want)
		}
	}
}

func TestReleaseWorkflowPublishesBothImagesWithoutLatest(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/release.yml")
	for _, want := range []string{
		"linux/amd64,linux/arm64",
		"ghcr.io/shikanon/orag-api",
		"ghcr.io/shikanon/orag-console",
		"docker/build-push-action@v6",
		"sbom: true",
		"provenance: mode=max",
		"cosign sign --yes",
		"gh release create",
		"--prerelease",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release workflow missing %q", want)
		}
	}
	if strings.Contains(workflow, ":latest") || strings.Contains(workflow, "type=raw,value=latest") {
		t.Error("prerelease workflow must not publish latest")
	}
}
