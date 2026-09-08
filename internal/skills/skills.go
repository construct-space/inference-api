package skills

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// A Skill is a piece of focused knowledge injected into the model's system
// prompt on demand. It is just a markdown file on disk; the router decides
// which ones to load per request.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body,omitempty"`
}

type Registry struct {
	dir   string
	mu    sync.RWMutex
	cache map[string]*Skill
}

func New(dir string) *Registry {
	return &Registry{dir: dir, cache: map[string]*Skill{}}
}

// List returns skill stubs (name + description only) — the menu the router
// shows the model.
func (r *Registry) List() ([]Skill, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Skill{}, nil
		}
		return nil, err
	}
	out := []Skill{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		s, err := r.Get(name)
		if err != nil {
			continue
		}
		out = append(out, Skill{Name: s.Name, Description: s.Description})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns a full skill (with body). Cached after first read.
func (r *Registry) Get(name string) (*Skill, error) {
	r.mu.RLock()
	if s, ok := r.cache[name]; ok {
		r.mu.RUnlock()
		return s, nil
	}
	r.mu.RUnlock()

	clean := filepath.Clean(name)
	if strings.ContainsAny(clean, "/\\") || clean == "." || clean == ".." {
		return nil, errors.New("invalid skill name")
	}
	path := filepath.Join(r.dir, clean+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := parse(clean, string(raw))

	r.mu.Lock()
	r.cache[clean] = s
	r.mu.Unlock()
	return s, nil
}

// parse extracts the first non-empty line as description, the rest as body.
// Skill files look like:
//
//	Nuxt 3 patterns and conventions.
//
//	## Routing
//	...
func parse(name, raw string) *Skill {
	s := &Skill{Name: name, Body: raw}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// skip a leading "# Title" line — take the line after it
		if strings.HasPrefix(line, "#") {
			continue
		}
		s.Description = line
		break
	}
	return s
}
