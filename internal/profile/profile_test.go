package profile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveAndLoadProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backend-local.toml")
	want := Profile{Version: Version, Name: "backend-local", Extends: "gradle.api.bootRun", EnvFrom: []string{".env.local"}, Env: map[string]string{"SERVER_PORT": "8081"}, PrependArgs: []string{"--stacktrace"}, AppendArgs: []string{"--args=--spring.profiles.active=local"}}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profile = %#v, want %#v", got, want)
	}
}

func TestResolveEnvFilesAndOverrides(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("API_HOST=localhost\nAPI_URL=http://${API_HOST}:8080 # comment\nQUOTED=\"hello world\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	values, err := ResolveEnv(root, []string{".env.local"}, map[string]string{"API_HOST": "127.0.0.1", "TOKEN": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if values["API_URL"] != "http://localhost:8080" || values["API_HOST"] != "127.0.0.1" || values["QUOTED"] != "hello world" {
		t.Fatalf("values = %#v", values)
	}
}

func TestFilenameRejectsPathTraversal(t *testing.T) {
	if _, err := Filename("../unsafe"); err == nil {
		t.Fatal("expected invalid profile name")
	}
}
