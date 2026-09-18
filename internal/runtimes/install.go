package runtimes

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	nodeIndexURL        = "https://nodejs.org/dist/index.json"
	adoptiumReleasesURL = "https://api.adoptium.net/v3/info/available_releases"
	goDownloadsURL      = "https://go.dev/dl/?mode=json&include=all"
	ltsInstallCount     = 5
)

// InstallRequest accepts runtime families instead of patches. Java and Node
// use an integer major; Go uses its language family (for example 1.26).
type InstallRequest struct {
	Runtime string
	Version string
	LTS     bool
	All     bool
}

type InstalledRuntime struct {
	Runtime string
	Family  string
	Version string
	Path    string
}

// AvailableVersion describes a release that can be selected for installation.
type AvailableVersion struct {
	Runtime string
	Version string
	LTS     bool
}

// Installer obtains release metadata and archives only from the runtime
// vendors' HTTPS endpoints. Client can be replaced in tests.
type Installer struct {
	Client *http.Client
	GOOS   string
	GOARCH string
}

func NewInstaller() Installer {
	return Installer{Client: &http.Client{Timeout: 2 * time.Minute}, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
}

// Install downloads and atomically places selected runtimes in LDR's shared
// store. It is intentionally macOS-only for this release.
func (i Installer) Install(ctx context.Context, request InstallRequest) ([]InstalledRuntime, error) {
	if i.GOOS != "darwin" {
		return nil, fmt.Errorf("runtime installation is currently supported on macOS only")
	}
	if i.Client == nil {
		i.Client = &http.Client{Timeout: 2 * time.Minute}
	}
	store, err := StoreRoot()
	if err != nil {
		return nil, err
	}
	if request.All {
		var installed []InstalledRuntime
		for _, name := range []string{"java", "node", "go"} {
			items, err := i.Install(ctx, InstallRequest{Runtime: name, LTS: request.LTS && name != "go"})
			if err != nil {
				return installed, err
			}
			installed = append(installed, items...)
		}
		return installed, nil
	}
	switch request.Runtime {
	case "node":
		return i.installNode(ctx, store, request)
	case "java":
		return i.installJava(ctx, store, request)
	case "go", "golang":
		return i.installGo(ctx, store, request)
	default:
		return nil, fmt.Errorf("unsupported runtime %q (choose java, node, or go)", request.Runtime)
	}
}

type archiveRelease struct {
	Runtime string
	Family  string
	Version string
	URL     string
	SHA256  string
	Home    string
}

type nodeRelease struct {
	Version string          `json:"version"`
	LTS     json.RawMessage `json:"lts"`
	Files   []string        `json:"files"`
}

// AvailableVersions returns stable/selectable releases for a runtime, newest
// first. The query is matched against the displayed version.
func (i Installer) AvailableVersions(ctx context.Context, runtimeName, query string) ([]AvailableVersion, error) {
	query = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(query, "v")))
	switch runtimeName {
	case "java":
		var available adoptiumReleases
		if err := i.getJSON(ctx, adoptiumReleasesURL, &available); err != nil {
			return nil, fmt.Errorf("get Java releases: %w", err)
		}
		lts := make(map[int]bool, len(available.AvailableLTSReleases))
		for _, major := range available.AvailableLTSReleases {
			lts[major] = true
		}
		versions := make([]AvailableVersion, 0, len(available.AvailableReleases))
		for _, major := range available.AvailableReleases {
			version := strconv.Itoa(major)
			if query != "" && !strings.Contains(version, query) {
				continue
			}
			versions = append(versions, AvailableVersion{Runtime: "java", Version: version, LTS: lts[major]})
		}
		sort.Slice(versions, func(a, b int) bool {
			left, _ := strconv.Atoi(versions[a].Version)
			right, _ := strconv.Atoi(versions[b].Version)
			return left > right
		})
		return versions, nil
	case "node":
		var releases []nodeRelease
		if err := i.getJSON(ctx, nodeIndexURL, &releases); err != nil {
			return nil, fmt.Errorf("get Node releases: %w", err)
		}
		versions := make([]AvailableVersion, 0, len(releases))
		seen := map[string]bool{}
		for _, release := range releases {
			version := strings.TrimPrefix(release.Version, "v")
			if !isStableNodeVersion(version) || seen[version] || (query != "" && !strings.Contains(strings.ToLower(version), query)) {
				continue
			}
			seen[version] = true
			versions = append(versions, AvailableVersion{Runtime: "node", Version: version, LTS: isNodeLTS(release.LTS)})
		}
		sort.SliceStable(versions, func(a, b int) bool { return nodeVersionLess(versions[b].Version, versions[a].Version) })
		return versions, nil
	default:
		return nil, fmt.Errorf("unsupported runtime %q; choose java or node", runtimeName)
	}
}

func isNodeLTS(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed != "" && trimmed != "false" && trimmed != "null"
}

func isStableNodeVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func nodeVersionLess(left, right string) bool {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for index := range leftParts {
		leftNumber, _ := strconv.Atoi(leftParts[index])
		rightNumber, _ := strconv.Atoi(rightParts[index])
		if leftNumber != rightNumber {
			return leftNumber < rightNumber
		}
	}
	return false
}

func (i Installer) installNode(ctx context.Context, store string, request InstallRequest) ([]InstalledRuntime, error) {
	if request.Version != "" && request.LTS {
		return nil, errors.New("choose either a Node major or --lts")
	}
	var releases []nodeRelease
	if err := i.getJSON(ctx, nodeIndexURL, &releases); err != nil {
		return nil, fmt.Errorf("get Node releases: %w", err)
	}
	selected := map[int]nodeRelease{}
	for _, release := range releases { // Node's index is newest first.
		major, err := versionMajor(release.Version)
		if err != nil || selected[major].Version != "" {
			continue
		}
		if request.Version != "" && request.Version != strconv.Itoa(major) {
			continue
		}
		if request.LTS && string(release.LTS) == "false" {
			continue
		}
		archiveName := "osx-arm64-tar"
		if i.GOARCH == "amd64" {
			archiveName = "osx-x64-tar"
		}
		if !containsString(release.Files, archiveName) {
			if request.LTS {
				continue
			}
			return nil, fmt.Errorf("Node %d has no macOS %s archive", major, i.GOARCH)
		}
		selected[major] = release
		if request.Version != "" || (!request.LTS && request.Version == "") {
			break
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no Node release matches %q", request.Version)
	}
	majors := sortedIntKeys(selected)
	if request.LTS && len(majors) > ltsInstallCount {
		majors = majors[len(majors)-ltsInstallCount:]
	}
	arch := "arm64"
	if i.GOARCH == "amd64" {
		arch = "x64"
	}
	var installed []InstalledRuntime
	for _, major := range majors {
		release := selected[major]
		file := "node-" + release.Version + "-darwin-" + arch + ".tar.gz"
		base := "https://nodejs.org/dist/" + release.Version + "/"
		checksum, err := i.nodeChecksum(ctx, base+"SHASUMS256.txt", file)
		if err != nil {
			return installed, err
		}
		item, err := i.installArchive(ctx, store, archiveRelease{Runtime: "node", Family: strconv.Itoa(major), Version: release.Version, URL: base + file, SHA256: checksum, Home: "bin/node"})
		if err != nil {
			return installed, err
		}
		if err := installPnpm(ctx, item.Path); err != nil {
			return installed, err
		}
		installed = append(installed, item)
	}
	return installed, nil
}

type adoptiumReleases struct {
	AvailableLTSReleases []int `json:"available_lts_releases"`
	AvailableReleases    []int `json:"available_releases"`
}

type adoptiumAsset struct {
	Binary struct {
		Package struct {
			Link     string `json:"link"`
			Checksum string `json:"checksum"`
		} `json:"package"`
	} `json:"binary"`
}

func (i Installer) installJava(ctx context.Context, store string, request InstallRequest) ([]InstalledRuntime, error) {
	if request.Version != "" && request.LTS {
		return nil, errors.New("choose either a Java major or --lts")
	}
	var available adoptiumReleases
	if err := i.getJSON(ctx, adoptiumReleasesURL, &available); err != nil {
		return nil, fmt.Errorf("get Java releases: %w", err)
	}
	majors := []int{}
	if request.Version != "" {
		major, err := strconv.Atoi(request.Version)
		if err != nil || major < 8 {
			return nil, fmt.Errorf("invalid Java major %q", request.Version)
		}
		majors = []int{major}
	} else if request.LTS {
		majors = append(majors, available.AvailableLTSReleases...)
		sort.Ints(majors)
		if len(majors) > ltsInstallCount {
			majors = majors[len(majors)-ltsInstallCount:]
		}
	} else {
		if len(available.AvailableReleases) == 0 {
			return nil, errors.New("no Java releases are available")
		}
		sort.Ints(available.AvailableReleases)
		majors = []int{available.AvailableReleases[len(available.AvailableReleases)-1]}
	}
	arch := "aarch64"
	if i.GOARCH == "amd64" {
		arch = "x64"
	}
	var installed []InstalledRuntime
	for _, major := range majors {
		endpoint := fmt.Sprintf("https://api.adoptium.net/v3/assets/latest/%d/hotspot?architecture=%s&image_type=jdk&os=mac&vendor=eclipse", major, arch)
		var assets []adoptiumAsset
		if err := i.getJSON(ctx, endpoint, &assets); err != nil {
			if request.LTS {
				continue
			}
			return installed, fmt.Errorf("get Java %d: %w", major, err)
		}
		if len(assets) == 0 || assets[0].Binary.Package.Link == "" || assets[0].Binary.Package.Checksum == "" {
			if request.LTS {
				continue
			}
			return installed, fmt.Errorf("Java %d has no macOS %s archive", major, arch)
		}
		item, err := i.installArchive(ctx, store, archiveRelease{Runtime: "java", Family: strconv.Itoa(major), Version: strconv.Itoa(major), URL: assets[0].Binary.Package.Link, SHA256: assets[0].Binary.Package.Checksum, Home: "Contents/Home/bin/java"})
		if err != nil {
			return installed, err
		}
		installed = append(installed, item)
	}
	return installed, nil
}

func installPnpm(ctx context.Context, nodeHome string) error {
	npm := filepath.Join(nodeHome, "bin", "npm")
	if _, err := os.Stat(npm); err != nil {
		return fmt.Errorf("Node package manager is unavailable at %s: %w", npm, err)
	}
	process := exec.CommandContext(ctx, npm, "install", "--global", "--prefix", nodeHome, "pnpm")
	process.Env = append(os.Environ(), "PATH="+filepath.Join(nodeHome, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	if output, err := process.CombinedOutput(); err != nil {
		return fmt.Errorf("install pnpm: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Stat(filepath.Join(nodeHome, "bin", "pnpm")); err != nil {
		return fmt.Errorf("pnpm installation did not create an executable: %w", err)
	}
	return nil
}

type goRelease struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Files   []struct {
		Filename string `json:"filename"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		Kind     string `json:"kind"`
		SHA256   string `json:"sha256"`
	} `json:"files"`
}

func (i Installer) installGo(ctx context.Context, store string, request InstallRequest) ([]InstalledRuntime, error) {
	if request.LTS {
		return nil, errors.New("Go has no LTS release line; use `ldr install go` or `ldr install go 1.26`")
	}
	if strings.EqualFold(request.Version, "latest") {
		request.Version = ""
	}
	releases, err := i.goReleases(ctx)
	if err != nil {
		return nil, err
	}
	var requested *GoVersion
	if request.Version != "" {
		version, err := ParseGoVersion(request.Version)
		if err != nil {
			return nil, fmt.Errorf("invalid Go version %q: %w", request.Version, err)
		}
		requested = &version
	}
	for _, release := range releases {
		version, err := ParseGoVersion(release.Version)
		if err != nil || (requested != nil && (version.Family() != requested.Family() || (requested.HasPatch && version.String() != requested.String()))) {
			continue
		}
		for _, file := range release.Files {
			if file.OS != "darwin" || file.Arch != i.GOARCH || file.Kind != "archive" {
				continue
			}
			item, err := i.installArchive(ctx, store, archiveRelease{Runtime: "go", Family: version.Family(), Version: release.Version, URL: "https://go.dev/dl/" + file.Filename, SHA256: file.SHA256, Home: "bin/go"})
			if err != nil {
				return nil, err
			}
			return []InstalledRuntime{item}, nil
		}
	}
	return nil, fmt.Errorf("no stable Go release matches %q", request.Version)
}

// AvailableGoVersions returns stable Go releases, newest first. A query may
// be a family (1.26), an exact version (1.26.3), or any text contained in the
// normalized version string.
func (i Installer) AvailableGoVersions(ctx context.Context, query string) ([]GoVersion, error) {
	releases, err := i.goReleases(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(query)), "go")
	versions := make([]GoVersion, 0, len(releases))
	for _, release := range releases {
		version, err := ParseGoVersion(release.Version)
		if err != nil || (query != "" && !strings.Contains(version.String(), query)) {
			continue
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func (i Installer) goReleases(ctx context.Context) ([]goRelease, error) {
	var releases []goRelease
	if err := i.getJSON(ctx, goDownloadsURL, &releases); err != nil {
		return nil, fmt.Errorf("get Go releases: %w", err)
	}
	stable := releases[:0]
	for _, release := range releases {
		if !release.Stable {
			continue
		}
		if _, err := ParseGoVersion(release.Version); err != nil {
			continue
		}
		stable = append(stable, release)
	}
	sort.SliceStable(stable, func(a, b int) bool {
		left, _ := ParseGoVersion(stable[a].Version)
		right, _ := ParseGoVersion(stable[b].Version)
		return goVersionLess(right, left)
	})
	return stable, nil
}

func goVersionLess(left, right GoVersion) bool {
	if left.Major != right.Major {
		return left.Major < right.Major
	}
	if left.Minor != right.Minor {
		return left.Minor < right.Minor
	}
	return left.Patch < right.Patch
}

func (i Installer) installArchive(ctx context.Context, store string, release archiveRelease) (InstalledRuntime, error) {
	archive, err := i.download(ctx, release.URL, release.SHA256)
	if err != nil {
		return InstalledRuntime{}, err
	}
	defer os.Remove(archive)
	parent := filepath.Join(store, release.Runtime)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return InstalledRuntime{}, err
	}
	extract, err := os.MkdirTemp("", "ldr-runtime-extract-*")
	if err != nil {
		return InstalledRuntime{}, err
	}
	defer os.RemoveAll(extract)
	if err := extractTarGz(archive, extract); err != nil {
		return InstalledRuntime{}, fmt.Errorf("extract %s: %w", release.Runtime, err)
	}
	home, err := findArchiveHome(extract, filepath.FromSlash(release.Home))
	if err != nil {
		return InstalledRuntime{}, err
	}
	stage, err := os.MkdirTemp(parent, ".ldr-install-")
	if err != nil {
		return InstalledRuntime{}, err
	}
	if err := copyTree(home, stage); err != nil {
		os.RemoveAll(stage)
		return InstalledRuntime{}, err
	}
	target := filepath.Join(parent, release.Family)
	backup := target + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		os.RemoveAll(stage)
		return InstalledRuntime{}, err
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			os.RemoveAll(stage)
			return InstalledRuntime{}, err
		}
	}
	if err := os.Rename(stage, target); err != nil {
		if _, backupErr := os.Stat(backup); backupErr == nil {
			_ = os.Rename(backup, target)
		}
		os.RemoveAll(stage)
		return InstalledRuntime{}, err
	}
	_ = os.RemoveAll(backup)
	return InstalledRuntime{Runtime: release.Runtime, Family: release.Family, Version: release.Version, Path: target}, nil
}

func (i Installer) getJSON(ctx context.Context, endpoint string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := i.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", endpoint, response.Status)
	}
	return json.NewDecoder(response.Body).Decode(output)
}

func (i Installer) nodeChecksum(ctx context.Context, endpoint, filename string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	response, err := i.Client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", endpoint, response.Status)
	}
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("Node checksum for %s was not found", filename)
}

func (i Installer) download(ctx context.Context, endpoint, checksum string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	response, err := i.Client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", endpoint, response.Status)
	}
	file, err := os.CreateTemp("", "ldr-runtime-*.tar.gz")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hash), response.Body); err != nil {
		file.Close()
		os.Remove(file.Name())
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return "", err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), strings.TrimSpace(checksum)) {
		os.Remove(file.Name())
		return "", fmt.Errorf("SHA-256 mismatch for %s", endpoint)
	}
	return file.Name(), nil
}

func extractTarGz(archive, destination string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		path := filepath.Join(destination, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			output, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode)&0o755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(output, reader); err != nil {
				output.Close()
				return err
			}
			if err := output.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), header.Linkname))
			relative, err := filepath.Rel(destination, resolved)
			if err != nil || filepath.IsAbs(header.Linkname) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return fmt.Errorf("unsafe archive symlink %q", header.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, path); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry %q", header.Name)
		}
	}
}

func findArchiveHome(root, executable string) (string, error) {
	var home string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != filepath.Base(executable) {
			return err
		}
		candidate, err := filepath.Rel(root, path)
		if err != nil || !strings.HasSuffix(filepath.ToSlash(candidate), filepath.ToSlash(executable)) {
			return err
		}
		// Every supported archive describes its executable below bin/. The
		// managed family root is the directory above that bin directory: this
		// flattens a macOS JDK's Contents/Home while preserving Node and Go
		// archive roots unchanged.
		home = filepath.Dir(filepath.Dir(path))
		return filepath.SkipAll
	})
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("runtime archive did not contain %s", executable)
	}
	return home, nil
}

func copyTree(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func versionMajor(value string) (int, error) {
	value = strings.TrimPrefix(value, "v")
	part := strings.Split(value, ".")[0]
	return strconv.Atoi(part)
}

func sortedIntKeys[T any](values map[int]T) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
