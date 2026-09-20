package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the optional project-local .ldr/project.toml configuration.
type Config struct {
	Version        int
	DefaultProfile string
	Ignore         []string
	Runtimes       map[string]string
}

func LoadConfig(root string) (Config, error) {
	path := filepath.Join(root, ".ldr", "project.toml")
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Version: 1, Runtimes: map[string]string{}}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	config := Config{Runtimes: map[string]string{}}
	section := ""
	for number, rawLine := range strings.Split(string(contents), "\n") {
		line := strings.TrimSpace(strings.SplitN(rawLine, "#", 2)[0])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return Config{}, fmt.Errorf("parse %s line %d", path, number+1)
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch section {
		case "":
			switch key {
			case "version":
				parsed, err := strconv.Atoi(value)
				if err != nil || parsed != 1 {
					return Config{}, fmt.Errorf("parse %s field version", path)
				}
				config.Version = parsed
			case "default_profile":
				parsed, err := strconv.Unquote(value)
				if err != nil {
					return Config{}, fmt.Errorf("parse %s field default_profile: %w", path, err)
				}
				config.DefaultProfile = parsed
			case "ignore":
				parsed, err := parseStringList(value)
				if err != nil {
					return Config{}, fmt.Errorf("parse %s field ignore: %w", path, err)
				}
				config.Ignore = parsed
			}
		case "runtimes":
			parsed, err := strconv.Unquote(value)
			if err != nil {
				return Config{}, fmt.Errorf("parse %s runtime %s: %w", path, key, err)
			}
			config.Runtimes[key] = parsed
		}
	}
	if config.Version == 0 {
		config.Version = 1
	}
	return config, nil
}

func parseStringList(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '[' || value[len(value)-1] != ']' {
		return nil, errors.New("expected string array")
	}
	value = strings.TrimSpace(value[1 : len(value)-1])
	if value == "" {
		return nil, nil
	}
	var result []string
	for _, item := range strings.Split(value, ",") {
		parsed, err := strconv.Unquote(strings.TrimSpace(item))
		if err != nil {
			return nil, err
		}
		result = append(result, parsed)
	}
	return result, nil
}
