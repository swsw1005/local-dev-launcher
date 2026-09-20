package search

import (
	"sort"
	"strings"
	"unicode"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

type Result struct {
	Task  domain.Task `json:"task"`
	Score int         `json:"score"`
}

// Tasks ranks case-insensitive fuzzy matches across task ID, name, module,
// adapter, and description. Exact token matches rank ahead of subsequences.
func Tasks(tasks []domain.Task, query string) []Result {
	query = strings.ToLower(strings.TrimSpace(query))
	results := make([]Result, 0, len(tasks))
	for _, task := range tasks {
		fields := []string{task.ID, task.Name, task.Module, task.Adapter, task.Description}
		best := 0
		for _, field := range fields {
			value := strings.ToLower(field)
			if query == "" {
				best = 1
				break
			}
			if value == query {
				best = max(best, 1000)
			} else if strings.Contains(value, query) {
				best = max(best, 700-len(value))
			} else if score := subsequenceScore(value, query); score > 0 {
				best = max(best, score)
			}
		}
		if best > 0 {
			results = append(results, Result{Task: task, Score: best})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Task.ID < results[j].Task.ID
	})
	return results
}

func subsequenceScore(value, query string) int {
	position, gaps := 0, 0
	for _, wanted := range query {
		found := false
		for position < len(value) {
			current := rune(value[position])
			position++
			if unicode.ToLower(current) == wanted {
				found = true
				break
			}
			gaps++
		}
		if !found {
			return 0
		}
	}
	return 400 - gaps
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
