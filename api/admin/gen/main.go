package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type patch struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

func main() {
	dir := "api/admin"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	spec, err := readJSON(filepath.Join(dir, "openapi.upstream.json"))
	if err != nil {
		fail(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "patches.json"))
	if err != nil {
		fail(err)
	}
	var patches []patch
	if err := json.Unmarshal(raw, &patches); err != nil {
		fail(err)
	}

	for index, p := range patches {
		if err := apply(spec, p); err != nil {
			fail(fmt.Errorf("patch %d (%s): %w", index, p.Path, err))
		}
	}

	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "openapi.json"), append(out, '\n'), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("applied %d patches to %s/openapi.json\n", len(patches), dir)
}

func apply(spec map[string]any, p patch) error {
	tokens, err := pointer(p.Path)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return fmt.Errorf("empty path")
	}

	parent, err := resolve(spec, tokens[:len(tokens)-1], p.Op == "set")
	if err != nil {
		return err
	}
	leaf := tokens[len(tokens)-1]

	switch p.Op {
	case "set":
		var value any
		if err := json.Unmarshal(p.Value, &value); err != nil {
			return err
		}
		return set(parent, leaf, value)
	case "remove":
		asObject, ok := parent.(map[string]any)
		if !ok {
			return fmt.Errorf("cannot remove %q from a non-object", leaf)
		}
		if _, ok := asObject[leaf]; !ok {
			return fmt.Errorf("nothing to remove at %q", p.Path)
		}
		delete(asObject, leaf)
		return nil
	default:
		return fmt.Errorf("unknown op %q", p.Op)
	}
}

func set(parent any, leaf string, value any) error {
	switch container := parent.(type) {
	case map[string]any:
		container[leaf] = value
		return nil
	case []any:
		index, err := strconv.Atoi(leaf)
		if err != nil {
			return fmt.Errorf("index %q into an array is not a number", leaf)
		}
		if index < 0 || index >= len(container) {
			return fmt.Errorf("index %d is out of range for an array of %d", index, len(container))
		}
		container[index] = value
		return nil
	default:
		return fmt.Errorf("cannot set %q on %T", leaf, parent)
	}
}

func resolve(node any, tokens []string, create bool) (any, error) {
	for i, token := range tokens {
		switch container := node.(type) {
		case map[string]any:
			next, ok := container[token]
			if !ok {
				if !create {
					return nil, fmt.Errorf("no such key %q at /%s", token, strings.Join(tokens[:i+1], "/"))
				}
				next = map[string]any{}
				container[token] = next
			}
			node = next
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil {
				return nil, fmt.Errorf("index %q at /%s is not a number", token, strings.Join(tokens[:i+1], "/"))
			}
			if index < 0 || index >= len(container) {
				return nil, fmt.Errorf("index %d at /%s is out of range", index, strings.Join(tokens[:i+1], "/"))
			}
			node = container[index]
		default:
			return nil, fmt.Errorf("cannot descend into %T at /%s", node, strings.Join(tokens[:i+1], "/"))
		}
	}
	return node, nil
}

func pointer(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("path %q must start with /", path)
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		part = strings.ReplaceAll(part, "~1", "/")
		parts[i] = strings.ReplaceAll(part, "~0", "~")
	}
	return parts, nil
}

func readJSON(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "spec patch failed:", err)
	os.Exit(1)
}
