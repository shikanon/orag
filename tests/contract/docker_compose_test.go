package contract

import (
	"regexp"
	"strings"
	"testing"
)

var immutableActionReference = regexp.MustCompile(`uses:\s+[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}\s+#\s+v\d`)

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
	if !strings.Contains(workflow, "docker/build-push-action@") || !immutableActionReference.MatchString(workflow) {
		t.Error("release workflow must use build-push-action through an immutable commit with a version comment")
	}
}

func TestWorkflowAndContainerInputsAreImmutable(t *testing.T) {
	for _, path := range []string{
		".github/workflows/ci.yml",
		".github/workflows/docs.yml",
		".github/workflows/release.yml",
	} {
		workflow := readRepoFile(t, path)
		for lineNumber, line := range strings.Split(workflow, "\n") {
			if strings.Contains(line, "uses:") && !immutableActionReference.MatchString(line) {
				t.Errorf("%s:%d action must use a full commit SHA and version comment: %s", path, lineNumber+1, strings.TrimSpace(line))
			}
		}
	}

	for _, path := range []string{"deployments/Dockerfile", "deployments/console.Dockerfile"} {
		dockerfile := readRepoFile(t, path)
		for lineNumber, line := range strings.Split(dockerfile, "\n") {
			if strings.HasPrefix(line, "FROM ") && !strings.Contains(line, "@sha256:") {
				t.Errorf("%s:%d base image must use an immutable digest: %s", path, lineNumber+1, line)
			}
		}
	}
}
