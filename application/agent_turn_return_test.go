// SPDX-License-Identifier: BUSL-1.1

package application

import (
	"context"
	"testing"

	"github.com/Koten-ai-org/zeus_client_golang/config"
	"github.com/Koten-ai-org/zeus_client_golang/domain"
	"github.com/Koten-ai-org/zeus_client_golang/ports"
)

func TestReturnEmptySummaryDoesNotCallZeus(t *testing.T) {
	for _, summary := range []string{"", "   ", "\n\t"} {
		llm := &scriptedLLM{script: []any{
			ports.LlmResponse{ToolCalls: []map[string]any{
				tc("return", returnArgs(map[string]any{"summary": summary}), "c1"),
			}},
		}}
		zeus := &scriptedZeus{}
		result := RunAgentTurn(context.Background(), TurnRequest{
			Message:  "list beers",
			Tools:    []map[string]any{returnTool},
			Settings: config.ClientSettings{AIProcessResult: false, MaxRounds: 2},
		}, RunAgentTurnOpts{LLM: llm, Zeus: zeus})
		if len(zeus.calls) != 0 {
			t.Fatalf("summary %q called zeus %+v", summary, zeus.calls)
		}
		if result.Debug.AIProcessResultExit != "cheap_terminal" {
			t.Fatalf("summary %q exit %q status %s", summary, result.Debug.AIProcessResultExit, result.Status)
		}
	}
}

func TestReturnResultDoesNotCallZeus(t *testing.T) {
	llm := &scriptedLLM{script: []any{
		ports.LlmResponse{ToolCalls: []map[string]any{
			tc("return_result", returnArgs(nil), "c1"),
		}},
	}}
	zeus := &scriptedZeus{}
	result := RunAgentTurn(context.Background(), TurnRequest{
		Message:  "list beers",
		Tools:    []map[string]any{returnTool},
		Settings: config.ClientSettings{AIProcessResult: false, MaxRounds: 2},
	}, RunAgentTurnOpts{LLM: llm, Zeus: zeus})
	if len(zeus.calls) != 0 {
		t.Fatalf("zeus calls %+v", zeus.calls)
	}
	if result.Status != TurnOK || result.Answer != "Found two beers in Tampa." {
		t.Fatalf("status %s answer %q err %+v", result.Status, result.Answer, result.Err)
	}
	if result.Debug.AIProcessResultExit != "cheap_terminal" {
		t.Fatalf("exit %q", result.Debug.AIProcessResultExit)
	}
}

func TestReturnTransportErrorKeepsCheapTerminal(t *testing.T) {
	llm := &scriptedLLM{script: []any{
		ports.LlmResponse{ToolCalls: []map[string]any{
			tc("return", returnArgs(map[string]any{"summary": "Kept after transport."}), "c1"),
		}},
	}}
	zeus := &scriptedZeus{errs: map[string]error{
		"return": domain.NewZeusTransport(domain.CodeZeusTransport, "zeus", domain.WithMessage("dial failed")),
	}}
	result := RunAgentTurn(context.Background(), TurnRequest{
		Message:  "list beers",
		Tools:    []map[string]any{returnTool},
		Settings: config.ClientSettings{AIProcessResult: false, MaxRounds: 2},
	}, RunAgentTurnOpts{LLM: llm, Zeus: zeus})
	if result.Status != TurnOK {
		t.Fatalf("status %s err %+v", result.Status, result.Err)
	}
	if result.Answer != "Kept after transport." {
		t.Fatalf("answer %q", result.Answer)
	}
	if result.Debug.AIProcessResultExit != "cheap_terminal" {
		t.Fatalf("exit %q", result.Debug.AIProcessResultExit)
	}
	if result.Err != nil {
		t.Fatalf("err %+v", result.Err)
	}
	if len(zeus.calls) != 1 || zeus.calls[0].Verb != "return" {
		t.Fatalf("zeus calls %+v", zeus.calls)
	}
	if len(result.Debug.Hops) != 1 || asString(result.Debug.Hops[0]["name"]) != "return" {
		t.Fatalf("hops %+v", result.Debug.Hops)
	}
	ok, _ := result.Debug.Hops[0]["ok"].(bool)
	if ok || asString(result.Debug.Hops[0]["error"]) == "" {
		t.Fatalf("hop %+v", result.Debug.Hops[0])
	}
}

func TestReturnCancelAndDeadlineAbortTurn(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code domain.Code
	}{
		{"cancel", domain.New(domain.CodeCancelled, "zeus", domain.WithMessage("cancelled")), domain.CodeCancelled},
		{"deadline", domain.New(domain.CodeContextDeadline, "zeus", domain.WithMessage("deadline")), domain.CodeContextDeadline},
	}
	for _, tcse := range cases {
		t.Run(tcse.name, func(t *testing.T) {
			llm := &scriptedLLM{script: []any{
				ports.LlmResponse{ToolCalls: []map[string]any{
					tc("return", returnArgs(map[string]any{"summary": "Should not finish."}), "c1"),
				}},
			}}
			zeus := &scriptedZeus{errs: map[string]error{"return": tcse.err}}
			result := RunAgentTurn(context.Background(), TurnRequest{
				Message:  "list beers",
				Tools:    []map[string]any{returnTool},
				Settings: config.ClientSettings{AIProcessResult: false, MaxRounds: 2},
			}, RunAgentTurnOpts{LLM: llm, Zeus: zeus})
			if result.Status != TurnError {
				t.Fatalf("status %s answer %q", result.Status, result.Answer)
			}
			if result.Err == nil || result.Err.Code != tcse.code {
				t.Fatalf("err %+v", result.Err)
			}
			if result.Answer == "Should not finish." {
				t.Fatal("cancelled return kept the summary")
			}
			if result.Debug.AIProcessResultExit == "cheap_terminal" {
				t.Fatal("exit cheap_terminal")
			}
		})
	}
}
