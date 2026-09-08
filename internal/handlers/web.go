package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"construct/inference/internal/together"
	"construct/inference/internal/web"
)

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func WebSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteJSON(w, 400, map[string]any{"error": "invalid json"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 25*time.Second)
	defer cancel()
	results, err := web.Search(ctx, body.Query, body.Limit)
	if err != nil {
		WriteJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	WriteJSON(w, 200, map[string]any{"results": results})
}

func WebFetch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
		Raw bool   `json:"raw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteJSON(w, 400, map[string]any{"error": "invalid json"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 25*time.Second)
	defer cancel()
	res, err := web.Fetch(ctx, body.URL, body.Raw)
	if err != nil {
		WriteJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	WriteJSON(w, 200, res)
}

// BuiltinTools returns OpenAI-shaped tool specs for tools the inference-api
// hosts directly (web search + fetch). The orchestrator merges these into
// the `tools` array it passes to /api/chat, then routes tool_calls named
// `web_search` / `web_fetch` back to /api/web/* on this service.
func BuiltinTools(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, 200, map[string]any{
		"tools": []together.Tool{
			{
				Type: "function",
				Function: together.ToolFunction{
					Name:        "web_search",
					Description: "Search the web. Returns up to 10 results with title, URL, and snippet. Use this when you need current information not in the training data. Follow up with web_fetch on promising URLs.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string", "description": "Search query."},
							"limit": map[string]any{"type": "integer", "description": "Max results (default 10, cap 20)."},
						},
						"required": []string{"query"},
					},
				},
			},
			{
				Type: "function",
				Function: together.ToolFunction{
					Name:        "web_fetch",
					Description: "Fetch a URL over HTTP(S). HTML pages are stripped to plain text. JSON and text responses are returned verbatim. 200KB cap; http/https only; 20s timeout.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"url": map[string]any{"type": "string", "description": "Full URL including scheme."},
							"raw": map[string]any{"type": "boolean", "description": "Return body verbatim even if HTML (default false)."},
						},
						"required": []string{"url"},
					},
				},
			},
		},
	})
}
