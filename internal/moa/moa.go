// Package moa orchestrates a Mixture-of-Agents request — multiple
// proposer models run in parallel and an aggregator synthesizes their
// outputs into the streamed reply. Wire shape matches the Together
// cookbook (https://docs.together.ai/docs/mixture-of-agents):
//
//  1. For each proposer, send the original message history with
//     stream=false; collect the assistant content.
//  2. Build an additional system message of the form
//
//	    <synthesisPrompt>
//	    Responses from models:
//	    1. <proposer 1 output>
//	    2. <proposer 2 output>
//	    ...
//
//  3. Send the aggregator the original messages + the synthesis
//     system message, stream=true, return the body so the caller
//     proxies SSE straight through to the end user.
//
// Failures: proposer errors are tolerated as long as at least one
// proposer succeeds. If all proposers fail the aggregator call is
// skipped and an error is returned.
package moa

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"construct/inference/internal/models"
	"construct/inference/internal/together"
)

// Run executes the proposer phase non-streaming and the aggregator
// phase non-streaming. Returned ChatResponse mirrors the aggregator's
// own response so the caller can pretend a single model produced it.
func Run(ctx context.Context, client *together.Client, recipe *models.MoA, base *together.ChatRequest) (*together.ChatResponse, error) {
	if client == nil || recipe == nil || base == nil {
		return nil, errors.New("moa: nil client, recipe, or base request")
	}
	aggReq, err := buildAggregatorRequest(ctx, client, recipe, base)
	if err != nil {
		return nil, err
	}
	aggReq.Stream = false
	return client.Chat(ctx, aggReq)
}

// Stream executes the proposer phase non-streaming and the aggregator
// phase streaming. Returns the SSE body for the caller to proxy.
func Stream(ctx context.Context, client *together.Client, recipe *models.MoA, base *together.ChatRequest) (io.ReadCloser, error) {
	if client == nil || recipe == nil || base == nil {
		return nil, errors.New("moa: nil client, recipe, or base request")
	}
	aggReq, err := buildAggregatorRequest(ctx, client, recipe, base)
	if err != nil {
		return nil, err
	}
	aggReq.Stream = true
	return client.Stream(ctx, aggReq)
}

// buildAggregatorRequest fans out to proposers in parallel, then
// constructs the aggregator's ChatRequest with the synthesis system
// message appended. Same shape as `base` otherwise — keeps temperature,
// tools, etc. that the caller chose.
func buildAggregatorRequest(ctx context.Context, client *together.Client, recipe *models.MoA, base *together.ChatRequest) (*together.ChatRequest, error) {
	if recipe.Aggregator == "" {
		return nil, errors.New("moa: aggregator model required")
	}
	if len(recipe.Proposers) == 0 {
		return nil, errors.New("moa: at least one proposer required")
	}

	outputs := fanOutProposers(ctx, client, recipe, base)
	if len(outputs) == 0 {
		return nil, errors.New("moa: all proposers failed; aggregator skipped")
	}

	// Synthesis system message goes FIRST, matching Together's MoA
	// cookbook pattern. A trailing system message (appended after
	// user/assistant turns) made some chat templates — DeepSeek and
	// Qwen variants in particular — return empty content because
	// the template expected the conversation to end with a user
	// turn followed by the assistant prefix. Putting the synthesis
	// up front gives the aggregator the proposer outputs as
	// "background" and lets the user message land at the right spot
	// in the template.
	synth := buildSynthesisMessage(recipe.SynthesisPrompt(), outputs)
	msgs := make([]together.Message, 0, len(base.Messages)+1)
	msgs = append(msgs, synth)
	msgs = append(msgs, base.Messages...)

	req := *base // shallow copy preserves temperature/top_p/tools
	req.Model = recipe.Aggregator
	req.Messages = msgs
	// Tools at the aggregator stage are typically wrong: proposers
	// already produced text content, the aggregator should synthesize
	// text, not re-invoke tools. Strip the tools array so the model
	// doesn't try to call tools against synthesized inputs.
	req.Tools = nil
	req.ToolChoice = nil
	return &req, nil
}

// proposerMinMaxTokens is the floor we apply to a proposer's
// max_tokens. Several Together models (DeepSeek V4 Pro, gpt-oss-120B,
// Kimi K2.6) are reasoning models — they spend tokens on internal
// `<think>` blocks before emitting visible content. If the caller
// passes a tight budget like 50, the entire allotment is consumed by
// reasoning and the proposer returns an empty string. Bumping the
// floor at the proposer phase ensures every proposer has enough room
// to think AND produce a usable synthesis input, regardless of what
// the end user passed for their final-answer cap.
const proposerMinMaxTokens = 2048

// fanOutProposers runs every proposer in parallel, non-streaming.
// Returns the list of successful assistant outputs in proposer order
// (failures elided — caller checks empty result for the all-failed
// case). Errors are logged to context-aware sinks elsewhere; this
// package stays quiet so the same code paths work in tests.
func fanOutProposers(ctx context.Context, client *together.Client, recipe *models.MoA, base *together.ChatRequest) []string {
	type result struct {
		idx     int
		content string
		err     error
	}
	results := make(chan result, len(recipe.Proposers))
	var wg sync.WaitGroup
	for i, model := range recipe.Proposers {
		wg.Add(1)
		go func(i int, model string) {
			defer wg.Done()
			req := *base
			req.Model = model
			req.Stream = false
			// Floor the proposer budget — see proposerMinMaxTokens.
			if req.MaxTokens < proposerMinMaxTokens {
				req.MaxTokens = proposerMinMaxTokens
			}
			// Proposer responses feed synthesis text; tool calls would
			// derail the pattern. Strip tools at the proposer stage
			// too.
			req.Tools = nil
			req.ToolChoice = nil
			resp, err := client.Chat(ctx, &req)
			if err != nil {
				results <- result{idx: i, err: err}
				return
			}
			if len(resp.Choices) == 0 {
				results <- result{idx: i, err: errors.New("no choices in proposer response")}
				return
			}
			results <- result{idx: i, content: resp.Choices[0].Message.Content}
		}(i, model)
	}
	wg.Wait()
	close(results)

	// Preserve proposer order so the synthesis prompt is stable
	// regardless of which call won the race.
	byIdx := make(map[int]string, len(recipe.Proposers))
	for r := range results {
		if r.err == nil && strings.TrimSpace(r.content) != "" {
			byIdx[r.idx] = r.content
		}
	}
	out := make([]string, 0, len(byIdx))
	for i := range recipe.Proposers {
		if c, ok := byIdx[i]; ok {
			out = append(out, c)
		}
	}
	return out
}

func buildSynthesisMessage(prompt string, outputs []string) together.Message {
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n")
	for i, o := range outputs {
		fmt.Fprintf(&b, "%d. %s\n\n", i+1, strings.TrimSpace(o))
	}
	return together.Message{
		Role:    "system",
		Content: strings.TrimRight(b.String(), "\n"),
	}
}
