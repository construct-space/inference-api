// Package agents loads agent definitions from disk. An agent is a
// markdown file with YAML frontmatter declaring its default model,
// pre-loaded skills, and recommended built-in tools. The body of the
// file is the agent's identity prompt — injected as a system message
// before any user messages.
package agents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Agent struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Model       string   `json:"model,omitempty"`
	Skills      []string `json:"skills,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	// Body is the agent's identity prompt (everything after frontmatter).
	Body string `json:"body,omitempty"`
}

type Registry struct {
	dir   string
	mu    sync.RWMutex
	cache map[string]*Agent
}

func New(dir string) *Registry {
	return &Registry{dir: dir, cache: map[string]*Agent{}}
}

// List returns agent stubs (no body) — the menu for /api/agents.
func (r *Registry) List() ([]Agent, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Agent{}, nil
		}
		return nil, err
	}
	out := []Agent{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		a, err := r.Get(name)
		if err != nil {
			continue
		}
		// Strip body for list view.
		stub := *a
		stub.Body = ""
		out = append(out, stub)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns a full agent (with body). Cached after first read.
func (r *Registry) Get(name string) (*Agent, error) {
	r.mu.RLock()
	if a, ok := r.cache[name]; ok {
		r.mu.RUnlock()
		return a, nil
	}
	r.mu.RUnlock()

	clean := filepath.Clean(name)
	if strings.ContainsAny(clean, "/\\") || clean == "." || clean == ".." {
		return nil, errors.New("invalid agent name")
	}
	path := filepath.Join(r.dir, clean+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	a, err := parse(clean, string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", clean, err)
	}

	r.mu.Lock()
	r.cache[clean] = a
	r.mu.Unlock()
	return a, nil
}

// parse expects YAML-ish frontmatter between --- markers, then the body.
// Frontmatter shape (all optional except name):
//
//	---
//	name: apoc
//	description: code-authoring operator
//	model: Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8
//	skills: [space-anatomy, construct-graph, actions, one-shot-space]
//	tools: [web_search, web_fetch]
//	temperature: 0.2
//	---
//
// We parse a minimal subset by hand to avoid pulling in yaml.v3 just for
// this — same pattern as the .env loader in config.go.
func parse(name, raw string) (*Agent, error) {
	a := &Agent{Name: name}
	body := raw

	if strings.HasPrefix(raw, "---") {
		// Find closing --- on its own line.
		rest := raw[3:]
		end := strings.Index(rest, "\n---")
		if end == -1 {
			return nil, errors.New("unterminated frontmatter")
		}
		fm := rest[:end]
		body = strings.TrimLeft(rest[end+4:], "\r\n")
		if err := parseFrontmatter(a, fm); err != nil {
			return nil, err
		}
	}

	a.Body = strings.TrimSpace(body)
	if a.Description == "" {
		// First non-empty body line as description fallback.
		for _, line := range strings.Split(a.Body, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			a.Description = line
			break
		}
	}
	return a, nil
}

func parseFrontmatter(a *Agent, fm string) error {
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		switch key {
		case "name":
			if val != "" {
				a.Name = val
			}
		case "description":
			a.Description = val
		case "model":
			a.Model = val
		case "skills":
			a.Skills = parseList(val)
		case "tools":
			a.Tools = parseList(val)
		case "temperature":
			f, err := parseFloat(val)
			if err == nil {
				a.Temperature = &f
			}
		}
	}
	return nil
}

func parseList(val string) []string {
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
