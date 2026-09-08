// Package models is the Construct-branded model catalog. It maps
// user-facing `construct-*` ids to upstream Together inference
// targets — either a single passthrough model or a Mixture-of-Agents
// recipe (multiple proposers synthesized by one aggregator, per
// https://docs.together.ai/docs/mixture-of-agents).
//
// The gateway's /models endpoint exposes this catalog so any caller
// (brain, browser tools, OpenAI SDKs) sees the Construct vocabulary
// rather than raw Together model identifiers. Resolution to the
// upstream id (or MoA recipe) happens inside Chat/ChatStream — the
// caller never has to know.
package models

import "slices"

// Model is one Construct-branded entry.
type Model struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Context     int      `json:"context,omitempty"`
	GoodFor     []string `json:"good_for,omitempty"`
	PriceInM    float64  `json:"price_in_M,omitempty"`
	PriceOutM   float64  `json:"price_out_M,omitempty"`
	CachedInM   float64  `json:"cached_in_M,omitempty"`
	Description string   `json:"description,omitempty"`

	// Resolution: exactly one of Upstream or MoA must be set.
	// Upstream is a Together model id — the request is forwarded
	// after rewriting the Model field. MoA fan-outs to multiple
	// proposers and synthesizes via an aggregator.
	Upstream string `json:"-"`
	MoA      *MoA   `json:"-"`
}

// MoA is a Mixture-of-Agents recipe.
//
// Proposers run in parallel as non-streaming completions; the
// aggregator receives their outputs as additional context and streams
// the final response to the caller. Synthesis prompt is taken from
// Together's reference implementation (with `synthesisPrompt` below
// as the default — Model.MoA.System overrides it per recipe).
type MoA struct {
	Proposers  []string // Together model ids
	Aggregator string   // Together model id — produces the final stream
	System     string   // optional synthesis-prompt override
}

// synthesisPrompt mirrors the reference MoA aggregator instruction
// from the Together cookbook. Quoted verbatim because the field tests
// across model families show measurable quality regressions when the
// wording drifts — kept here so individual recipes can override only
// when there's a real reason to.
const synthesisPrompt = `You have been provided with a set of responses from various open-source models to the latest user query. Your task is to synthesize these responses into a single, high-quality response. Critically evaluate the information provided in these responses, recognizing that some of it may be biased or incorrect. Your response should not simply replicate the given answers but should offer a refined, accurate, and comprehensive reply to the instruction. Ensure your response is well-structured, coherent, and adheres to the highest standards of accuracy and reliability.

Responses from models:`

// SynthesisPrompt returns the aggregator instruction for the recipe,
// falling back to the catalog default when the recipe doesn't
// override it.
func (m *MoA) SynthesisPrompt() string {
	if m == nil {
		return ""
	}
	if m.System != "" {
		return m.System
	}
	return synthesisPrompt
}

// Catalog is the set of Construct-branded models the gateway exposes.
// Deliberately a single entry: end users don't pick a tier. The server
// routes inside via Tank → operator dispatch (Trinity / Apoc / Mouse /
// Oracle / Neo) and falls through to Morpheus MoA for hard prompts.
// The picker only exists so users on another provider can switch back
// to Construct from the provider dropdown — there is no model choice
// to make beyond "use Construct".
var Catalog = []Model{
	{
		ID:          "source",
		Label:       "Source",
		Context:     262_000,
		Description: "Mixture of Agents — auto-routed across the Source family (Tank, Trinity, Apoc, Mouse, Oracle, Neo, Morpheus). 100 free prompts per user.",
		GoodFor:     []string{"default", "code", "design", "hard-reasoning", "vision", "agent-loops"},
		// Blended price reflecting the typical mix Tank routes to. Real
		// per-call cost depends on which operator answers; quoted price
		// is the upper bound (Morpheus MoA pro path).
		PriceInM:  6.00,
		PriceOutM: 10.00,
		// Today: until inference-api fetches the source_family routing
		// table from provider-api, the underlying recipe is the Morpheus
		// MoA the previous "construct-pro" used. Tank dispatch lives
		// in the planned source-family runtime; this keeps the picker
		// usable in the meantime.
		MoA: &MoA{
			Proposers: []string{
				"Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8",
				"openai/gpt-oss-120B",
				"moonshotai/Kimi-K2.6",
			},
			Aggregator: "deepseek-ai/DeepSeek-V4-Pro",
		},
	},
}

// Default is the model id returned in the catalog response and used
// when callers don't specify a model. Picks the one entry tagged
// "default" in GoodFor; if none match, falls back to the first
// entry.
func Default() string {
	for _, m := range Catalog {
		if slices.Contains(m.GoodFor, "default") {
			return m.ID
		}
	}
	if len(Catalog) > 0 {
		return Catalog[0].ID
	}
	return ""
}

// Lookup returns the catalog entry for `id`, or nil if the id isn't
// in the catalog (passthrough to Together with the original id).
func Lookup(id string) *Model {
	for i := range Catalog {
		if Catalog[i].ID == id {
			return &Catalog[i]
		}
	}
	return nil
}

// Resolve maps an incoming model id to (upstream id, MoA recipe).
// Returns:
//
//   - upstream != "", moa == nil  → passthrough; rewrite the Model
//     field to upstream and forward the request as-is.
//   - upstream == "", moa != nil  → MoA orchestration; caller routes
//     through moa.Run / moa.Stream.
//   - upstream == "", moa == nil  → unknown to the catalog; treat as
//     a raw Together id and forward unchanged (preserves backward
//     compat with callers still using "Qwen/Qwen3.6-Plus"-style ids).
func Resolve(id string) (upstream string, moa *MoA) {
	m := Lookup(id)
	if m == nil {
		return "", nil
	}
	if m.MoA != nil {
		return "", m.MoA
	}
	return m.Upstream, nil
}
