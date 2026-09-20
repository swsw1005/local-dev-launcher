package search

import (
	"testing"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

func TestTasksRanksFuzzyMatchesDeterministically(t *testing.T) {
	results := Tasks([]domain.Task{{ID: "gradle.api.bootRun", Name: "bootRun", Module: "api"}, {ID: "node.web.dev", Name: "dev", Module: "web"}}, "boot")
	if len(results) != 1 || results[0].Task.ID != "gradle.api.bootRun" {
		t.Fatalf("results = %#v", results)
	}
}
