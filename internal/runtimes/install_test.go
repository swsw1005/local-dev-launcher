package runtimes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLinuxInstallerSelectsVendorArchives(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, name := range []string{"java", "node", "go"} {
			t.Run(name+"/"+arch, func(t *testing.T) {
				t.Setenv("LDR_RUNTIME_HOME", t.TempDir())
				nodeArch, javaArch := "arm64", "aarch64"
				if arch == "amd64" {
					nodeArch, javaArch = "x64", "x64"
				}
				seen := false
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					body, status := "", http.StatusOK
					switch {
					case r.URL.String() == nodeIndexURL:
						body = fmt.Sprintf(`[{"version":"v24.1.0","lts":"x","files":["linux-%s-tar"]}]`, nodeArch)
					case strings.HasSuffix(r.URL.Path, "SHASUMS256.txt"):
						body = fmt.Sprintf("%064d  node-v24.1.0-linux-%s.tar.gz\n", 0, nodeArch)
					case r.URL.String() == adoptiumReleasesURL:
						body = `{"available_releases":[21]}`
					case strings.Contains(r.URL.Path, "/assets/latest/"):
						if r.URL.Query().Get("os") != "linux" || r.URL.Query().Get("architecture") != javaArch {
							t.Fatalf("wrong Java request: %s", r.URL)
						}
						body = `[{"binary":{"package":{"link":"https://example.test/jdk.tar.gz","checksum":"00"}}}]`
					case r.URL.String() == goDownloadsURL:
						body = fmt.Sprintf(`[{"version":"go1.26.1","stable":true,"files":[{"os":"linux","arch":%q,"kind":"archive","filename":"go1.26.1.linux-%s.tar.gz","sha256":"00"}]}]`, arch, arch)
					default:
						if name == "node" && !strings.HasSuffix(r.URL.Path, "node-v24.1.0-linux-"+nodeArch+".tar.gz") {
							t.Fatalf("wrong Node archive: %s", r.URL)
						}
						if name == "go" && !strings.HasSuffix(r.URL.Path, ".linux-"+arch+".tar.gz") {
							t.Fatalf("wrong Go archive: %s", r.URL)
						}
						seen, status = true, http.StatusNotFound
					}
					return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
				})}
				_, err := (Installer{Client: client, GOOS: "linux", GOARCH: arch}).Install(context.Background(), InstallRequest{Runtime: name})
				if err == nil || !seen {
					t.Fatalf("expected archive download, got %v", err)
				}
			})
		}
	}
}

func TestInstallerRejectsUnsupportedPlatforms(t *testing.T) {
	for _, installer := range []Installer{{GOOS: "windows", GOARCH: "amd64"}, {GOOS: "linux", GOARCH: "386"}} {
		if _, err := installer.Install(context.Background(), InstallRequest{Runtime: "go"}); err == nil {
			t.Fatalf("expected platform error for %#v", installer)
		}
	}
}

func TestFindArchiveHomeForLinuxJava(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "jdk-21", "bin", "java")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	home, err := findArchiveHome(root, "bin/java")
	if err != nil || home != filepath.Join(root, "jdk-21") {
		t.Fatalf("home = %q, error = %v", home, err)
	}
}
