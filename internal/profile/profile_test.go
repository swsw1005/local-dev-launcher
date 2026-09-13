package profile

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveAndLoadProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backend-local.toml")
	want := Profile{Version: Version, Name: "backend-local", Extends: "gradle.api.bootRun", Env: map[string]string{"SERVER_PORT": "8081"}, PrependArgs: []string{"--stacktrace"}, AppendArgs: []string{"--args=--spring.profiles.active=local"}}
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

func TestFilenameRejectsPathTraversal(t *testing.T) {
	if _, err := Filename("../unsafe"); err == nil {
		t.Fatal("expected invalid profile name")
	}
}
