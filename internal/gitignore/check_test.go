package gitignore

import (
	"context"
	"errors"
	"testing"
)

type fakeChecker struct {
	inRepository bool
	ignored      bool
	err          error
}

func (f fakeChecker) InsideWorkTree(context.Context, string) (bool, error) {
	return f.inRepository, f.err
}

func (f fakeChecker) IsIgnored(context.Context, string, string) (bool, error) {
	return f.ignored, f.err
}

func TestCheckOutsideGitRepository(t *testing.T) {
	status, err := Check(context.Background(), fakeChecker{}, "/project")
	if err != nil {
		t.Fatal(err)
	}
	if status.InRepository || status.Ignored {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestCheckIgnoredDirectory(t *testing.T) {
	status, err := Check(context.Background(), fakeChecker{inRepository: true, ignored: true}, "/project")
	if err != nil {
		t.Fatal(err)
	}
	if !status.InRepository || !status.Ignored {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestCheckPropagatesGitErrors(t *testing.T) {
	want := errors.New("git missing")
	_, err := Check(context.Background(), fakeChecker{err: want}, "/project")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
