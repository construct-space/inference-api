package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"construct/inference/internal/agents"
	"construct/inference/internal/capture"
	"construct/inference/internal/config"
	"construct/inference/internal/moa"
	"construct/inference/internal/models"
	"construct/inference/internal/skills"
	"construct/inference/internal/together"

	"github.com/google/uuid"
)

var (
	Cfg     *config.Config
	Capture *capture.Logger

	togetherOnce  bool
	togetherCli   *together.Client
	skillRegistry *skills.Registry
	agentRegistry *agents.Registry
)

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func client() *together.Client {
	if !togetherOnce {
		togetherCli = together.New(Cfg.TogetherBaseURL, Cfg.TogetherAPIKey)
		togetherOnce = true
	}
	return togetherCli
}

func registry() *skills.Registry {
	if skillRegistry == nil {
		skillRegistry = skills.New(Cfg.SkillsDir)
	}
	return skillRegistry
}

func agentReg() *agents.Registry {
	if agentRegistry == nil {
		agentRegistry = agents.New(Cfg.AgentsDir)
	}
	return agentRegistry
}

// ChatBody is the inference-api's own request shape. Thin wrapper around the
// OpenAI shape so we can layer skill-loading and routing on top later.
type ChatBody struct {
	// Agent: name of an agent in agents/ — preloads its identity body,
	// skills, default model, and temperature. Explicit fields below override.
	Agent string `json:"agent,omitempty"`

	Model       string             `json:"model,omitempty"`
	Messages    []together.Message `json:"messages"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stop        []string           `json:"stop,omitempty"`

	// Skills: names of skill files to inject as system messages, in order,
	// before the caller's own messages. Merged with the agent's default
	// skills if both are set.
	Skills []string `json:"skills,omitempty"`

	// Tools: OpenAI-shaped function tools passed through to the upstream.
	// ToolChoice is "auto" | "none" | { "type": "function", "function": { "name": "..." } }.
	Tools      []together.Tool `json:"tools,omitempty"`
	ToolChoice any             `json:"tool_choice,omitempty"`

	// Capture: opt-out per request. Default true (capture everything).
	// Set to false for sensitive flows (passwords, PII). A pointer so we
	// can distinguish "not set" from "explicitly false".
	Capture *bool `json:"capture,omitempty"`
}

// resolved is the side-channel info returned alongside the upstream
// request — needed for the capture record (skills actually loaded,
// agent body, etc.).
type resolved struct {
	skillNames []string
}

func (b *ChatBody) toRequest(cfg *config.Config, sreg *skills.Registry, areg *agents.Registry, stream bool) (*together.ChatRequest, *resolved, error) {
	model := b.Model
	temp := b.Temperature
	skillNames := b.Skills

	// Resolve agent: identity body becomes first system message; agent's
	// default skills are prepended to the skill list; model + temperature
	// fill in when the caller didn't set them explicitly.
	var agentBody string
	if b.Agent != "" {
		a, err := areg.Get(b.Agent)
		if err != nil {
			return nil, nil, fmt.Errorf("agent %q: %w", b.Agent, err)
		}
		agentBody = a.Body
		if model == "" {
			model = a.Model
		}
		if temp == nil {
			temp = a.Temperature
		}
		// Agent skills go FIRST; caller-supplied skills layer on top.
		merged := make([]string, 0, len(a.Skills)+len(skillNames))
		seen := map[string]bool{}
		for _, s := range a.Skills {
			if !seen[s] {
				merged = append(merged, s)
				seen[s] = true
			}
		}
		for _, s := range skillNames {
			if !seen[s] {
				merged = append(merged, s)
				seen[s] = true
			}
		}
		skillNames = merged
	}

	if model == "" {
		model = cfg.DefaultModel
	}
	maxTok := b.MaxTokens
	if maxTok <= 0 || maxTok > cfg.MaxOutputTokens {
		maxTok = cfg.MaxOutputTokens
	}

	msgs := make([]together.Message, 0, len(b.Messages)+len(skillNames)+1)
	if agentBody != "" {
		msgs = append(msgs, together.Message{Role: "system", Content: agentBody})
	}
	for _, name := range skillNames {
		s, err := sreg.Get(name)
		if err != nil {
			return nil, nil, fmt.Errorf("skill %q: %w", name, err)
		}
		msgs = append(msgs, together.Message{Role: "system", Content: s.Body})
	}
	msgs = append(msgs, b.Messages...)

	return &together.ChatRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTok,
		Temperature: temp,
		TopP:        b.TopP,
		Stop:        b.Stop,
		Stream:      stream,
		Tools:       b.Tools,
		ToolChoice:  b.ToolChoice,
	}, &resolved{skillNames: skillNames}, nil
}

func Chat(w http.ResponseWriter, r *http.Request) {
	var body ChatBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteJSON(w, 400, map[string]any{"error": "invalid json"})
		return
	}
	if len(body.Messages) == 0 {
		WriteJSON(w, 400, map[string]any{"error": "messages required"})
		return
	}
	req, res, err := body.toRequest(Cfg, registry(), agentReg(), false)
	if err != nil {
		WriteJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), together.PingTimeout)
	defer cancel()

	rec := newRecord(&body, req, res, false, r)
	start := time.Now()
	// Resolve a Construct-branded model id to either a Together
	// passthrough or an MoA recipe before dispatching.
	upstream, moaRecipe := models.Resolve(req.Model)
	var resp *together.ChatResponse
	switch {
	case moaRecipe != nil:
		resp, err = moa.Run(ctx, client(), moaRecipe, req)
	default:
		if upstream != "" {
			req.Model = upstream
		}
		resp, err = client().Chat(ctx, req)
	}
	rec.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		rec.OK = false
		rec.Error = err.Error()
		captureRecord(&body, rec)
		WriteJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	together.NormalizeResponse(resp)
	rec.OK = true
	rec.UpstreamRespID = resp.ID
	if len(resp.Choices) > 0 {
		c := resp.Choices[0]
		rec.Response = map[string]any{
			"content":       c.Message.Content,
			"tool_calls":    c.Message.ToolCalls,
			"finish_reason": c.FinishReason,
		}
	}
	rec.Usage = map[string]any{
		"prompt_tokens":     resp.Usage.PromptTokens,
		"completion_tokens": resp.Usage.CompletionTokens,
		"total_tokens":      resp.Usage.TotalTokens,
	}
	captureRecord(&body, rec)
	WriteJSON(w, 200, resp)
}

func ChatStream(w http.ResponseWriter, r *http.Request) {
	var body ChatBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteJSON(w, 400, map[string]any{"error": "invalid json"})
		return
	}
	if len(body.Messages) == 0 {
		WriteJSON(w, 400, map[string]any{"error": "messages required"})
		return
	}
	req, res, err := body.toRequest(Cfg, registry(), agentReg(), true)
	if err != nil {
		WriteJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteJSON(w, 500, map[string]any{"error": "streaming not supported"})
		return
	}
	rec := newRecord(&body, req, res, true, r)
	start := time.Now()
	// Same Construct-id resolution as the non-streaming path: MoA
	// recipes orchestrate proposers + aggregator (aggregator streams
	// back); passthrough rewrites the Model field.
	upstream, moaRecipe := models.Resolve(req.Model)
	var stream io.ReadCloser
	switch {
	case moaRecipe != nil:
		stream, err = moa.Stream(r.Context(), client(), moaRecipe, req)
	default:
		if upstream != "" {
			req.Model = upstream
		}
		stream, err = client().Stream(r.Context(), req)
	}
	if err != nil {
		rec.DurationMS = time.Since(start).Milliseconds()
		rec.OK = false
		rec.Error = err.Error()
		captureRecord(&body, rec)
		WriteJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher.Flush()

	var raw bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, rerr := stream.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			flusher.Flush()
			raw.Write(buf[:n])
		}
		if rerr != nil {
			rec.DurationMS = time.Since(start).Milliseconds()
			rec.OK = rerr == io.EOF
			if !rec.OK {
				rec.Error = rerr.Error()
			}
			content, toolCalls, usage := parseSSE(raw.Bytes())
			// If the model emitted tool calls inline as text, lift them
			// into the structured field so capture records see the same
			// shape regardless of backend.
			if len(toolCalls) == 0 {
				cleaned, lifted := together.ExtractInlineToolCalls(content)
				if len(lifted) > 0 {
					content = cleaned
					for _, tc := range lifted {
						toolCalls = append(toolCalls, tc)
					}
				}
			}
			rec.Response = map[string]any{
				"content":    content,
				"tool_calls": toolCalls,
			}
			if usage != nil {
				rec.Usage = usage
			}
			captureRecord(&body, rec)
			return
		}
	}
}

// parseSSE walks a streamed `data: {...}` blob and assembles the final
// content + tool_calls + usage. Best-effort: if the upstream uses a
// non-standard shape, we still record what came through verbatim.
func parseSSE(b []byte) (content string, toolCalls []any, usage map[string]any) {
	var contentBuf bytes.Buffer
	for _, line := range bytes.Split(b, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []any  `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(payload, &chunk); err != nil {
			continue
		}
		for _, c := range chunk.Choices {
			contentBuf.WriteString(c.Delta.Content)
			if len(c.Delta.ToolCalls) > 0 {
				toolCalls = append(toolCalls, c.Delta.ToolCalls...)
			}
		}
		if chunk.Usage != nil {
			usage = map[string]any{
				"prompt_tokens":     chunk.Usage.PromptTokens,
				"completion_tokens": chunk.Usage.CompletionTokens,
				"total_tokens":      chunk.Usage.TotalTokens,
			}
		}
	}
	return contentBuf.String(), toolCalls, usage
}

func newRecord(body *ChatBody, req *together.ChatRequest, res *resolved, stream bool, r *http.Request) *capture.Record {
	caller := map[string]any{}
	if v := r.Header.Get("X-User-ID"); v != "" {
		caller["user_id"] = v
	}
	if v := r.Header.Get("X-Org-ID"); v != "" {
		caller["org_id"] = v
	}
	if v := r.Header.Get("X-Source"); v != "" {
		caller["source"] = v
	}
	if len(caller) == 0 {
		caller = nil
	}
	var toolsOffered []any
	if len(req.Tools) > 0 {
		toolsOffered = capture.MustMessages(req.Tools)
	}
	return &capture.Record{
		ID:           uuid.NewString(),
		TS:           time.Now().UTC(),
		Agent:        body.Agent,
		Model:        req.Model,
		SkillsLoaded: res.skillNames,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		Stream:       stream,
		Messages:     capture.MustMessages(req.Messages),
		ToolsOffered: toolsOffered,
		Caller:       caller,
	}
}

func captureRecord(body *ChatBody, rec *capture.Record) {
	if Capture == nil {
		return
	}
	if body.Capture != nil && !*body.Capture {
		return
	}
	Capture.Append(rec)
}

// ListModels returns the Construct-branded model catalog. Each entry's
// id is the public name (`construct-pro`, `construct-fast`, …); the
// underlying Together id (single passthrough) or MoA recipe stays
// internal to this service and is resolved per-request inside
// Chat/ChatStream.
func ListModels(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, 200, map[string]any{
		"default":   models.Default(),
		"models":    models.Catalog,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func ListAgents(w http.ResponseWriter, r *http.Request) {
	list, err := agentReg().List()
	if err != nil {
		WriteJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	WriteJSON(w, 200, map[string]any{"agents": list})
}

func GetAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	a, err := agentReg().Get(name)
	if err != nil {
		WriteJSON(w, 404, map[string]any{"error": "agent not found"})
		return
	}
	WriteJSON(w, 200, a)
}

func ListSkills(w http.ResponseWriter, r *http.Request) {
	list, err := registry().List()
	if err != nil {
		WriteJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	WriteJSON(w, 200, map[string]any{"skills": list})
}

func GetSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s, err := registry().Get(name)
	if err != nil {
		WriteJSON(w, 404, map[string]any{"error": "skill not found"})
		return
	}
	WriteJSON(w, 200, s)
}
