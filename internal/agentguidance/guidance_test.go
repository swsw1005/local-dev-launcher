package agentguidance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearAgentEnv clears every environment variable anyEnvironmentSet inspects,
// so tests control agent detection deterministically regardless of the
// environment the test binary itself runs in.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"CODEX_HOME", "CODEX_THREAD_ID", "CODEX_SESSION_ID",
		"CLAUDECODE", "CLAUDE_CODE", "CLAUDE_PROJECT_DIR",
	} {
		t.Setenv(name, "")
	}
}

func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func findFile(t *testing.T, files []File, name string) File {
	t.Helper()
	for _, f := range files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no file named %q in %#v", name, files)
	return File{}
}

func TestApplyCreatesMissingFiles(t *testing.T) {
	clearAgentEnv(t)
	home := withHome(t)

	files, err := Apply(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}
	for _, f := range files {
		if f.Err != nil {
			t.Fatalf("%s: unexpected error %v", f.Name, f.Err)
		}
		if !f.Added || !f.HasGuidance || f.Damaged {
			t.Fatalf("%s: got Added=%v HasGuidance=%v Damaged=%v", f.Name, f.Added, f.HasGuidance, f.Damaged)
		}
		content, readErr := os.ReadFile(f.Path)
		if readErr != nil {
			t.Fatalf("%s: %v", f.Name, readErr)
		}
		if !strings.Contains(string(content), f.Marker) || !strings.Contains(string(content), f.EndMarker) {
			t.Fatalf("%s: written content missing markers: %q", f.Name, content)
		}
	}
	guide := findFile(t, files, "LDR runtime guide")
	if guide.Path != filepath.Join(home, ".ldr", "ldr_runtime_guide.md") {
		t.Fatalf("guide path = %q", guide.Path)
	}
}

func TestApplyIsIdempotentWhenBlockAlreadyPresent(t *testing.T) {
	clearAgentEnv(t)
	withHome(t)

	if _, err := Apply(false); err != nil {
		t.Fatal(err)
	}
	before, err := GlobalFiles()
	if err != nil {
		t.Fatal(err)
	}
	beforeContent := map[string][]byte{}
	for _, f := range before {
		content, err := os.ReadFile(f.Path)
		if err != nil {
			t.Fatal(err)
		}
		beforeContent[f.Path] = content
	}

	files, err := Apply(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Added || f.Repaired || f.Damaged || !f.HasGuidance {
			t.Fatalf("%s: expected no-op re-apply, got Added=%v Repaired=%v Damaged=%v HasGuidance=%v", f.Name, f.Added, f.Repaired, f.Damaged, f.HasGuidance)
		}
		content, err := os.ReadFile(f.Path)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != string(beforeContent[f.Path]) {
			t.Fatalf("%s: content changed on idempotent re-apply", f.Name)
		}
	}
}

func TestApplyAppendsBlockPreservingExistingContent(t *testing.T) {
	clearAgentEnv(t)
	home := withHome(t)

	codexPath := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(codexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "# My AGENTS.md\n\nSome unrelated project instructions.\n"
	if err := os.WriteFile(codexPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Apply(false)
	if err != nil {
		t.Fatal(err)
	}
	codex := findFile(t, files, "Codex global AGENTS.md link")
	if !codex.Added {
		t.Fatalf("expected Added=true, got %#v", codex)
	}
	content, err := os.ReadFile(codexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), existing) {
		t.Fatalf("existing content not preserved as prefix: %q", content)
	}
	if !strings.Contains(string(content), LinkMarker) {
		t.Fatalf("appended content missing marker: %q", content)
	}
}

func TestInspectDetectsDuplicatedMarkers(t *testing.T) {
	clearAgentEnv(t)
	home := withHome(t)

	guidePath := filepath.Join(home, ".ldr", "ldr_runtime_guide.md")
	if err := os.MkdirAll(filepath.Dir(guidePath), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := GuideMarker + "\nfirst\n" + GuideEndMarker + "\n" + GuideMarker + "\nsecond\n" + GuideEndMarker + "\n"
	if err := os.WriteFile(guidePath, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := GlobalFiles()
	if err != nil {
		t.Fatal(err)
	}
	guide := findFile(t, files, "LDR runtime guide")
	if guide.Problem == "" || guide.Repairable {
		t.Fatalf("expected unresolvable duplicated-marker problem, got %#v", guide)
	}
	if !guide.Damaged {
		t.Fatalf("expected Damaged=true, got %#v", guide)
	}
}

func TestInspectDetectsOutOfOrderMarkers(t *testing.T) {
	clearAgentEnv(t)
	home := withHome(t)

	guidePath := filepath.Join(home, ".ldr", "ldr_runtime_guide.md")
	if err := os.MkdirAll(filepath.Dir(guidePath), 0o755); err != nil {
		t.Fatal(err)
	}
	// End marker appears before the start marker.
	broken := GuideEndMarker + "\n" + GuideMarker + "\n"
	if err := os.WriteFile(guidePath, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := GlobalFiles()
	if err != nil {
		t.Fatal(err)
	}
	guide := findFile(t, files, "LDR runtime guide")
	if guide.Problem == "" || guide.Repairable {
		t.Fatalf("expected unresolvable out-of-order problem, got %#v", guide)
	}
}

func TestApplyWithoutForceLeavesDamagedBlockUnchanged(t *testing.T) {
	clearAgentEnv(t)
	home := withHome(t)

	guidePath := filepath.Join(home, ".ldr", "ldr_runtime_guide.md")
	if err := os.MkdirAll(filepath.Dir(guidePath), 0o755); err != nil {
		t.Fatal(err)
	}
	damaged := "before\n" + GuideMarker + "\nstale content\n" + GuideEndMarker + "\nafter\n"
	if err := os.WriteFile(guidePath, []byte(damaged), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Apply(false)
	if err != nil {
		t.Fatal(err)
	}
	guide := findFile(t, files, "LDR runtime guide")
	if !guide.Damaged || !guide.Repairable || guide.Repaired {
		t.Fatalf("expected damaged+repairable, not repaired, got %#v", guide)
	}
	content, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != damaged {
		t.Fatalf("file changed without force: %q", content)
	}
}

func TestApplyWithForceRepairsDamagedBlockPreservingSurroundingContent(t *testing.T) {
	clearAgentEnv(t)
	home := withHome(t)

	guidePath := filepath.Join(home, ".ldr", "ldr_runtime_guide.md")
	if err := os.MkdirAll(filepath.Dir(guidePath), 0o755); err != nil {
		t.Fatal(err)
	}
	damaged := "before-text\n" + GuideMarker + "\nstale content\n" + GuideEndMarker + "\nafter-text\n"
	if err := os.WriteFile(guidePath, []byte(damaged), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Apply(true)
	if err != nil {
		t.Fatal(err)
	}
	guide := findFile(t, files, "LDR runtime guide")
	if !guide.Repaired || guide.Damaged {
		t.Fatalf("expected repaired, got %#v", guide)
	}
	content, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "before-text\n") {
		t.Fatalf("prefix not preserved: %q", content)
	}
	if !strings.HasSuffix(string(content), "after-text\n") {
		t.Fatalf("suffix not preserved: %q", content)
	}
	if strings.Contains(string(content), "stale content") {
		t.Fatalf("stale content not replaced: %q", content)
	}
	if !strings.Contains(string(content), strings.TrimSpace(guideBlock)) {
		t.Fatalf("canonical block not spliced in: %q", content)
	}
}

func TestApplyWithForcePreservesFilePermissions(t *testing.T) {
	clearAgentEnv(t)
	home := withHome(t)

	guidePath := filepath.Join(home, ".ldr", "ldr_runtime_guide.md")
	if err := os.MkdirAll(filepath.Dir(guidePath), 0o755); err != nil {
		t.Fatal(err)
	}
	damaged := GuideMarker + "\nstale\n" + GuideEndMarker + "\n"
	if err := os.WriteFile(guidePath, []byte(damaged), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Apply(true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v, want 0600", info.Mode().Perm())
	}
}

func TestApplySkipsLinkFilesWhenGuideDamagedBeyondRepair(t *testing.T) {
	clearAgentEnv(t)
	home := withHome(t)

	guidePath := filepath.Join(home, ".ldr", "ldr_runtime_guide.md")
	if err := os.MkdirAll(filepath.Dir(guidePath), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := GuideMarker + "\na\n" + GuideEndMarker + "\n" + GuideMarker + "\nb\n" + GuideEndMarker + "\n"
	if err := os.WriteFile(guidePath, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	codexPath := filepath.Join(home, ".codex", "AGENTS.md")

	files, err := Apply(true)
	if err != nil {
		t.Fatal(err)
	}
	guide := findFile(t, files, "LDR runtime guide")
	if guide.Problem == "" {
		t.Fatalf("expected guide problem, got %#v", guide)
	}
	if _, statErr := os.Stat(codexPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected Codex file left untouched, stat err = %v", statErr)
	}
}

func TestDetectedAgentFilesEnvDetection(t *testing.T) {
	clearAgentEnv(t)
	withHome(t)

	if got := DetectedAgentFiles(); len(got) != 0 {
		t.Fatalf("with no agent env set, got %d files, want 0", len(got))
	}

	t.Setenv("CLAUDECODE", "1")
	got := DetectedAgentFiles()
	if len(got) != 2 {
		t.Fatalf("with Claude Code detected, got %d files, want 2 (link + guide)", len(got))
	}
	findFile(t, got, "Claude Code global CLAUDE.md link")
	findFile(t, got, "LDR runtime guide")

	t.Setenv("CLAUDECODE", "")
	t.Setenv("CODEX_HOME", "/some/path")
	got = DetectedAgentFiles()
	if len(got) != 2 {
		t.Fatalf("with Codex detected, got %d files, want 2 (link + guide)", len(got))
	}
	findFile(t, got, "Codex global AGENTS.md link")
}

func TestGlobalFilesAlwaysReturnsAllThree(t *testing.T) {
	clearAgentEnv(t)
	withHome(t)

	files, err := GlobalFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}
	for _, f := range files {
		if f.Exists {
			t.Fatalf("%s: expected Exists=false on empty home", f.Name)
		}
	}
}
