// Package capture records every chat request + response to local
// JSONL for later analysis and fine-tune dataset building. A
// background goroutine ships finalized day-files to R2 (S3-compatible)
// so multi-node deploys converge in one bucket.
//
// Design choices:
//
//   - Local-first: a network blip never drops a record. The local
//     file IS the source of truth; R2 is the durable mirror.
//   - Async: the request path drops a record on a buffered channel
//     and returns. The writer goroutine handles disk I/O.
//   - Daily rotation: one file per UTC date. R2 uploader ships only
//     completed days (current day stays local, flushed continuously).
//   - Capture-everything default: callers opt out per-request with
//     `capture: false`. A CAPTURE_ENABLED=false env flag kills the
//     whole subsystem (e.g. for tests).
package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Record is the wire shape persisted to JSONL. One line per request.
type Record struct {
	ID            string         `json:"id"`
	TS            time.Time      `json:"ts"`
	DurationMS    int64          `json:"duration_ms"`
	Agent         string         `json:"agent,omitempty"`
	Model         string         `json:"model"`
	SkillsLoaded  []string       `json:"skills_loaded,omitempty"`
	Temperature   *float64       `json:"temperature,omitempty"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Stream        bool           `json:"stream"`
	Messages      []any          `json:"messages"`
	ToolsOffered  []any          `json:"tools_offered,omitempty"`
	Response      map[string]any `json:"response,omitempty"`
	Usage         map[string]any `json:"usage,omitempty"`
	OK            bool           `json:"ok"`
	Error         string         `json:"error,omitempty"`
	Caller        map[string]any `json:"caller,omitempty"`
	UpstreamRespID string        `json:"upstream_response_id,omitempty"`
}

// Logger persists records. Safe for concurrent use.
type Logger struct {
	enabled bool
	dir     string

	mu      sync.Mutex
	day     string // current "2026-05-17" file we have open
	file    *os.File

	ch     chan *Record
	closed chan struct{}

	uploader *R2Uploader // optional
}

type Options struct {
	Enabled  bool
	Dir      string
	Buffer   int // channel buffer, default 256
	Uploader *R2Uploader
}

func New(opts Options) (*Logger, error) {
	if !opts.Enabled {
		return &Logger{enabled: false}, nil
	}
	if opts.Dir == "" {
		return nil, errors.New("capture: Dir is required when Enabled")
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("capture: mkdir %s: %w", opts.Dir, err)
	}
	buf := opts.Buffer
	if buf <= 0 {
		buf = 256
	}
	l := &Logger{
		enabled:  true,
		dir:      opts.Dir,
		ch:       make(chan *Record, buf),
		closed:   make(chan struct{}),
		uploader: opts.Uploader,
	}
	go l.run()
	if l.uploader != nil {
		go l.uploader.run(l.dir)
	}
	return l, nil
}

// Append drops the record on the writer channel. Never blocks the caller;
// if the buffer is full it logs and drops (data loss is preferable to
// stalling a chat response).
func (l *Logger) Append(r *Record) {
	if l == nil || !l.enabled || r == nil {
		return
	}
	select {
	case l.ch <- r:
	default:
		log.Printf("capture: buffer full, dropping record %s", r.ID)
	}
}

// Close drains the channel, flushes, closes the file.
func (l *Logger) Close() {
	if l == nil || !l.enabled {
		return
	}
	close(l.ch)
	<-l.closed
}

func (l *Logger) run() {
	for r := range l.ch {
		if err := l.write(r); err != nil {
			log.Printf("capture: write %s: %v", r.ID, err)
		}
	}
	l.mu.Lock()
	if l.file != nil {
		_ = l.file.Sync()
		_ = l.file.Close()
	}
	l.mu.Unlock()
	close(l.closed)
}

func (l *Logger) write(r *Record) error {
	day := r.TS.UTC().Format("2006-01-02")
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil || l.day != day {
		if l.file != nil {
			_ = l.file.Sync()
			_ = l.file.Close()
		}
		path := filepath.Join(l.dir, day+".jsonl")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		l.file = f
		l.day = day
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := l.file.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// ListDayFiles returns the daily JSONL files in the log dir, sorted.
// Used by the uploader to find finalized days.
func ListDayFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".jsonl") {
			continue
		}
		// Only date-shaped names (YYYY-MM-DD.jsonl).
		base := strings.TrimSuffix(n, ".jsonl")
		if _, err := time.Parse("2006-01-02", base); err != nil {
			continue
		}
		out = append(out, filepath.Join(dir, n))
	}
	sort.Strings(out)
	return out, nil
}

// CurrentDay returns today's UTC date in YYYY-MM-DD form. Used by the
// uploader to skip the active day.
func CurrentDay() string { return time.Now().UTC().Format("2006-01-02") }

// helper used by handler code — kept here so handlers don't reach into
// json themselves
func MustMessages(v any) []any {
	b, _ := json.Marshal(v)
	var out []any
	_ = json.Unmarshal(b, &out)
	return out
}

// MarshalContext converts an arbitrary value to a JSON-friendly map.
func MarshalContext(v any) map[string]any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// (Background context kept exported in case the uploader pkg wants it.)
var _ = context.Background
