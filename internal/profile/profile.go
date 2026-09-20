// Package profile persists user-owned execution profiles below .ldr/profiles.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const Version = 1

type Profile struct {
	Version     int
	Name        string
	Extends     string
	EnvFrom     []string
	Env         map[string]string
	PrependArgs []string
	AppendArgs  []string
}

func Filename(name string) (string, error) {
	if name == "" {
		return "", errors.New("profile name is empty")
	}
	for _, character := range name {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '-' && character != '_' && character != '.' {
			return "", fmt.Errorf("invalid profile name %q", name)
		}
	}
	return name + ".toml", nil
}

func Create(directory, name, taskID string) (Profile, error) {
	filename, err := Filename(name)
	if err != nil {
		return Profile{}, err
	}
	path := filepath.Join(directory, filename)
	if _, err := os.Lstat(path); err == nil {
		return Profile{}, fmt.Errorf("profile %q already exists", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Profile{}, err
	}
	profile := Profile{Version: Version, Name: name, Extends: taskID, Env: map[string]string{}}
	if err := Save(path, profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func Save(path string, profile Profile) error {
	if profile.Version == 0 {
		profile.Version = Version
	}
	if profile.Name == "" || profile.Extends == "" {
		return errors.New("profile requires name and extends")
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "version = %d\nname = %s\nextends = %s\n", profile.Version, strconv.Quote(profile.Name), strconv.Quote(profile.Extends))
	if len(profile.EnvFrom) > 0 {
		fmt.Fprintf(&builder, "env_from = %s\n", quotedList(profile.EnvFrom))
	}
	if len(profile.Env) > 0 {
		builder.WriteString("\n[env]\n")
		keys := make([]string, 0, len(profile.Env))
		for key := range profile.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&builder, "%s = %s\n", key, strconv.Quote(profile.Env[key]))
		}
	}
	if len(profile.PrependArgs) > 0 || len(profile.AppendArgs) > 0 {
		builder.WriteString("\n[args]\n")
		if len(profile.PrependArgs) > 0 {
			fmt.Fprintf(&builder, "prepend = %s\n", quotedList(profile.PrependArgs))
		}
		if len(profile.AppendArgs) > 0 {
			fmt.Fprintf(&builder, "append = %s\n", quotedList(profile.AppendArgs))
		}
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func Load(path string) (Profile, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	profile := Profile{Env: map[string]string{}}
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
			return Profile{}, fmt.Errorf("parse %s line %d", path, number+1)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch section {
		case "":
			switch key {
			case "version":
				parsed, err := strconv.Atoi(value)
				if err != nil {
					return Profile{}, fmt.Errorf("parse %s version: %w", path, err)
				}
				profile.Version = parsed
			case "name":
				profile.Name, err = strconv.Unquote(value)
				if err != nil {
					return Profile{}, fmt.Errorf("parse %s name: %w", path, err)
				}
			case "extends":
				profile.Extends, err = strconv.Unquote(value)
				if err != nil {
					return Profile{}, fmt.Errorf("parse %s extends: %w", path, err)
				}
			case "env_from":
				profile.EnvFrom, err = parseList(value)
				if err != nil {
					return Profile{}, fmt.Errorf("parse %s env_from: %w", path, err)
				}
			}
		case "env":
			parsed, err := strconv.Unquote(value)
			if err != nil {
				return Profile{}, fmt.Errorf("parse %s env %s: %w", path, key, err)
			}
			profile.Env[key] = parsed
		case "args":
			parsed, err := parseList(value)
			if err != nil {
				return Profile{}, fmt.Errorf("parse %s args %s: %w", path, key, err)
			}
			if key == "prepend" {
				profile.PrependArgs = parsed
			} else if key == "append" {
				profile.AppendArgs = parsed
			}
		}
	}
	if profile.Version != Version || profile.Name == "" || profile.Extends == "" {
		return Profile{}, fmt.Errorf("invalid profile %s", path)
	}
	return profile, nil
}

func LoadByName(directory, name string) (Profile, error) {
	filename, err := Filename(name)
	if err != nil {
		return Profile{}, err
	}
	return Load(filepath.Join(directory, filename))
}

func LoadAll(directory string) ([]Profile, error) {
	paths, err := filepath.Glob(filepath.Join(directory, "*.toml"))
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(paths))
	for _, path := range paths {
		profile, err := Load(path)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func quotedList(values []string) string {
	encoded, _ := json.Marshal(values)
	return string(encoded)
}

func parseList(value string) ([]string, error) {
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil, err
	}
	return values, nil
}

// ResolveEnv loads env files in declaration order and applies profile values
// last. Relative paths are resolved from the project root.
func ResolveEnv(root string, envFrom []string, overrides map[string]string) (map[string]string, error) {
	values := make(map[string]string)
	lookup := func(key string) string {
		if value, ok := values[key]; ok {
			return value
		}
		return os.Getenv(key)
	}
	for _, filename := range envFrom {
		path := filename
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		loaded, err := loadEnvFile(path)
		if err != nil {
			return nil, err
		}
		for key, value := range loaded {
			values[key] = os.Expand(value, lookup)
		}
	}
	for key, value := range overrides {
		values[key] = os.Expand(value, lookup)
	}
	return values, nil
}

func loadEnvFile(path string) (map[string]string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load env file %s: %w", path, err)
	}
	values := make(map[string]string)
	for number, rawLine := range strings.Split(string(contents), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("parse env file %s line %d", path, number+1)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			if value[0] == '"' {
				unquoted, err := strconv.Unquote(value)
				if err != nil {
					return nil, fmt.Errorf("parse env file %s line %d: %w", path, number+1, err)
				}
				value = unquoted
			} else {
				value = value[1 : len(value)-1]
			}
		} else if comment := strings.Index(value, " #"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		values[key] = value
	}
	return values, nil
}
