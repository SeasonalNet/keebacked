package buildinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseVersionIsConsistent(t *testing.T) {
	root := filepath.Join("..", "..")
	checks := map[string][]string{
		filepath.Join(root, "README.md"): {
			"Release versions are derived from Git `v*` tags",
			"keebacked:$(make image-tag)",
		},
		filepath.Join(root, ".env.example"): {
			"KEEBACKED_IMAGE=keebacked:dev",
			"KEEBACKED_VERSION=dev",
			"KEEBACKED_SOURCE_URL=",
		},
		filepath.Join(root, "CONTRIBUTING.md"): {
			"Git `vMAJOR.MINOR.PATCH` tags are the release version authority",
			"Create an annotated `vX.Y.Z` Git tag",
			"keebacked:$(make image-tag)",
		},
		filepath.Join(root, "compose.yaml"): {
			"KEEBACKED_VERSION:-dev",
			"KEEBACKED_IMAGE:-keebacked:dev",
			"SOURCE_URL: ${KEEBACKED_SOURCE_URL:-",
		},
		filepath.Join(root, "Makefile"): {
			"VERSION ?= $(strip $(shell sh tools/version.sh",
			"IMAGE_TAG ?= $(strip $(shell sh tools/version.sh --image-tag",
			"image: version-check",
		},
		filepath.Join(root, "tools", "version.sh"): {
			"--release)",
			"git_cmd describe --tags --abbrev=0",
			"dirty=.dirty",
		},
		filepath.Join(root, "Dockerfile"): {
			"org.opencontainers.image.version",
			"org.opencontainers.image.revision",
			"org.opencontainers.image.created",
		},
	}

	for path, needles := range checks {
		contents := readRepositoryFile(t, path)
		for _, needle := range needles {
			if !strings.Contains(contents, needle) {
				t.Errorf("%s does not contain %q", filepath.Base(path), needle)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, "VERSION")); !os.IsNotExist(err) {
		t.Fatal("manual VERSION file must not remain the release authority")
	}
}

func TestDeploymentExamplesAreHostNeutral(t *testing.T) {
	root := filepath.Join("..", "..")
	paths := []string{
		filepath.Join(root, "compose.yaml"),
		filepath.Join(root, ".env.example"),
		filepath.Join(root, "config", "keebacked.toml"),
	}
	for _, path := range paths {
		contents := strings.ToLower(readRepositoryFile(t, path))
		for _, forbidden := range []string{"/home/", "/mnt/", "${home}", "victus", "seasonalnas", "seasonals_hdd"} {
			if strings.Contains(contents, forbidden) {
				t.Errorf("%s contains host-specific value %q", filepath.Base(path), forbidden)
			}
		}
	}
}

func TestQualityWorkflowsProvisionNodeCompatibleGo(t *testing.T) {
	root := filepath.Join("..", "..")
	checks := map[string][]string{
		filepath.Join(root, ".forgejo", "workflows", "quality.yml"): {
			"name: Quality",
			"runs-on: docker",
			"uses: actions/checkout@v6",
			"uses: actions/setup-go@v6",
			"go-version-file: go.mod",
			"cache: false",
			"github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0",
			`PATH="$PWD/.bin:$PATH" make quality`,
		},
		filepath.Join(root, ".github", "workflows", "quality.yml"): {
			"name: Quality",
			"runs-on: ubuntu-latest",
			"uses: actions/checkout@v6",
			"uses: actions/setup-go@v6",
			"go-version-file: go.mod",
			"cache: false",
			"github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0",
			`PATH="$PWD/.bin:$PATH" make quality`,
		},
	}

	for path, needles := range checks {
		contents := readRepositoryFile(t, path)
		for _, needle := range needles {
			if !strings.Contains(contents, needle) {
				t.Errorf("%s does not contain %q", path, needle)
			}
		}
		if strings.Contains(contents, "container: golang:") {
			t.Errorf("%s replaces the Node-capable runner image with a Go-only job container", path)
		}
	}
}

func TestSecurityScanningConfigurationIsPresent(t *testing.T) {
	root := filepath.Join("..", "..")
	checks := map[string][]string{
		filepath.Join(root, ".forgejo", "workflows", "security.yml"): {
			"name: Security",
			"runs-on: docker",
			"GITLEAKS_VERSION: 8.30.1",
			"uses: actions/checkout@v6",
			"test -f .gitleaks.toml",
			"gitleaks dir . --config .gitleaks.toml --redact --verbose --no-banner",
		},
		filepath.Join(root, ".gitleaks.toml"): {
			`title = "SeasonalNet Gitleaks configuration"`,
			"useDefault = true",
			`regexTarget = "line"`,
			"DISCORD",
		},
	}

	for path, needles := range checks {
		contents := readRepositoryFile(t, path)
		for _, needle := range needles {
			if !strings.Contains(contents, needle) {
				t.Errorf("%s does not contain %q", path, needle)
			}
		}
	}
}

func readRepositoryFile(t *testing.T, path string) string {
	t.Helper()
	// #nosec G304 -- paths are fixed repository test fixtures.
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
