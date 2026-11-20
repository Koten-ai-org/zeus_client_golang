// SPDX-License-Identifier: BUSL-1.1

package domain

import (
	"reflect"
	"testing"
)

func peachBody() map[string]any {
	return map[string]any{
		"steps": []any{
			map[string]any{
				"as":          "peach_beers",
				"verb":        "search",
				"entity_type": "Beer",
				"strategy":    "fts",
				"query_text":  "peach",
			},
			map[string]any{
				"as":     "ranked",
				"verb":   "project",
				"ids":    "@peach_beers.ids",
				"fields": []any{"name", "style"},
			},
		},
		"summary": "keep @peach_beers.ids in the summary",
	}
}

func TestFTSProjectIDsBecomeDocKeys(t *testing.T) {
	body, changed := RewriteFTSIdsToDocKeys(peachBody())
	if !reflect.DeepEqual(changed, []string{"peach_beers"}) {
		t.Fatalf("changed %v", changed)
	}
	steps := body["steps"].([]any)
	project := steps[1].(map[string]any)
	if project["ids"] != "@peach_beers.doc_keys" {
		t.Fatalf("ids %v", project["ids"])
	}
	if body["summary"] != "keep @peach_beers.ids in the summary" {
		t.Fatalf("summary %v", body["summary"])
	}
	search := steps[0].(map[string]any)
	if search["strategy"] != "fts" {
		t.Fatalf("strategy %v", search["strategy"])
	}
}

func TestOrderPassthroughRewritesTheNextIDs(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "peach_beers", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{"as": "ranked", "verb": "order", "ids": "@peach_beers.ids", "by": "id"},
			map[string]any{"as": "rows", "verb": "project", "ids": "@ranked.ids", "fields": []any{"name"}},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if !reflect.DeepEqual(changed, []string{"peach_beers", "ranked"}) {
		t.Fatalf("changed %v", changed)
	}
	steps := out["steps"].([]any)
	if steps[1].(map[string]any)["ids"] != "@peach_beers.doc_keys" {
		t.Fatalf("order ids %v", steps[1])
	}
	if steps[2].(map[string]any)["ids"] != "@ranked.doc_keys" {
		t.Fatalf("project ids %v", steps[2])
	}
}

func TestHybridIDsStay(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "hits", "verb": "search", "strategy": "hybrid", "query_text": "peach"},
			map[string]any{"as": "ranked", "verb": "project", "ids": "@hits.ids"},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if len(changed) != 0 {
		t.Fatalf("changed %v", changed)
	}
	steps := out["steps"].([]any)
	if steps[1].(map[string]any)["ids"] != "@hits.ids" {
		t.Fatalf("ids %v", steps[1])
	}
}

func TestNodeIDsAndDocKeysStay(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "hits", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{
				"as":       "ranked",
				"verb":     "project",
				"ids":      "@hits.node_ids",
				"src_keys": "@hits.doc_keys",
			},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if len(changed) != 0 {
		t.Fatalf("changed %v", changed)
	}
	project := out["steps"].([]any)[1].(map[string]any)
	if project["ids"] != "@hits.node_ids" || project["src_keys"] != "@hits.doc_keys" {
		t.Fatalf("%v", project)
	}
}

func TestNestedParamsAndLongerStepName(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{"as": "peach_beers", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{"verb": "project", "params": map[string]any{"ids": []any{"@peach.ids", "@peach_beers.ids"}}},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if !reflect.DeepEqual(changed, []string{"peach", "peach_beers"}) {
		t.Fatalf("changed %v", changed)
	}
	params := out["steps"].([]any)[2].(map[string]any)["params"].(map[string]any)
	got := params["ids"].([]any)
	if got[0] != "@peach.doc_keys" || got[1] != "@peach_beers.doc_keys" {
		t.Fatalf("%v", got)
	}
}

func TestStrategyInsideParams(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "hits", "verb": "search", "params": map[string]any{"strategy": "fts", "query_text": "peach"}},
			map[string]any{"as": "rows", "verb": "project", "ids": "@hits.ids"},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if !reflect.DeepEqual(changed, []string{"hits"}) {
		t.Fatalf("changed %v", changed)
	}
	if out["steps"].([]any)[1].(map[string]any)["ids"] != "@hits.doc_keys" {
		t.Fatalf("%v", out["steps"])
	}
}

func TestNoStepsIsUnchanged(t *testing.T) {
	body := map[string]any{"summary": "hi"}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if len(changed) != 0 {
		t.Fatalf("changed %v", changed)
	}
	if !reflect.DeepEqual(out, body) {
		t.Fatalf("%v", out)
	}
}

func TestRewriteDoesNotMutateTheInput(t *testing.T) {
	body := peachBody()
	_, _ = RewriteFTSIdsToDocKeys(body)
	project := body["steps"].([]any)[1].(map[string]any)
	if project["ids"] != "@peach_beers.ids" {
		t.Fatalf("input mutated %v", project["ids"])
	}
}

func TestFetchedOrderKeepsTheFollowingGet(t *testing.T) {
	for _, by := range []string{"field:name", "name", "created_at"} {
		t.Run(by, func(t *testing.T) {
			body := map[string]any{
				"steps": []any{
					map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
					map[string]any{"as": "ranked", "verb": "order", "ids": "@peach.ids", "by": by},
					map[string]any{"as": "rows", "verb": "get", "ids": "@ranked.ids"},
				},
			}
			out, changed := RewriteFTSIdsToDocKeys(body)
			if !reflect.DeepEqual(changed, []string{"peach"}) {
				t.Fatalf("changed %v", changed)
			}
			steps := out["steps"].([]any)
			if steps[1].(map[string]any)["ids"] != "@peach.doc_keys" {
				t.Fatalf("order %v", steps[1])
			}
			if steps[1].(map[string]any)["by"] != by {
				t.Fatalf("by %v", steps[1])
			}
			if steps[2].(map[string]any)["ids"] != "@ranked.ids" {
				t.Fatalf("get %v", steps[2])
			}
		})
	}
	already := map[string]any{
		"steps": []any{
			map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{"as": "ranked", "verb": "order", "ids": "@peach.doc_keys", "by": "field:name"},
			map[string]any{"as": "rows", "verb": "get", "ids": "@ranked.ids"},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(already)
	if len(changed) != 0 {
		t.Fatalf("changed %v", changed)
	}
	if out["steps"].([]any)[2].(map[string]any)["ids"] != "@ranked.ids" {
		t.Fatalf("%v", out["steps"])
	}
}

func TestOrderByIDFollowsScoredDocKeys(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{"as": "ranked", "verb": "order", "ids": "@peach.scored", "by": "id"},
			map[string]any{"as": "rows", "verb": "project", "ids": "@ranked.ids"},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if !reflect.DeepEqual(changed, []string{"ranked"}) {
		t.Fatalf("changed %v", changed)
	}
	steps := out["steps"].([]any)
	if steps[1].(map[string]any)["ids"] != "@peach.scored" {
		t.Fatalf("order %v", steps[1])
	}
	if steps[2].(map[string]any)["ids"] != "@ranked.doc_keys" {
		t.Fatalf("project %v", steps[2])
	}
}

func TestGetTraverseAndNarrowToKeepIDs(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{"as": "walk", "verb": "traverse", "from": "@peach.ids"},
			map[string]any{"as": "found", "verb": "find", "narrow_to": "@peach.ids"},
			map[string]any{
				"as":        "card",
				"verb":      "project",
				"ids":       "@peach.ids",
				"narrow_to": "@peach.ids",
			},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if !reflect.DeepEqual(changed, []string{"peach"}) {
		t.Fatalf("changed %v", changed)
	}
	steps := out["steps"].([]any)
	if steps[1].(map[string]any)["from"] != "@peach.ids" {
		t.Fatalf("traverse %v", steps[1])
	}
	if steps[2].(map[string]any)["narrow_to"] != "@peach.ids" {
		t.Fatalf("find %v", steps[2])
	}
	project := steps[3].(map[string]any)
	if project["ids"] != "@peach.doc_keys" || project["narrow_to"] != "@peach.ids" {
		t.Fatalf("project %v", project)
	}
}

func TestSetNodeIDsKeepsAMixedUnion(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "cities", "verb": "find", "entity_type": "City"},
			map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{
				"as":      "both",
				"verb":    "set",
				"op":      "union",
				"id_kind": "node_ids",
				"inputs":  []any{"@cities.ids", "@peach.ids"},
			},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if len(changed) != 0 {
		t.Fatalf("changed %v", changed)
	}
	set := out["steps"].([]any)[2].(map[string]any)
	if !reflect.DeepEqual(set["inputs"], []any{"@cities.ids", "@peach.ids"}) || set["id_kind"] != "node_ids" {
		t.Fatalf("%v", set)
	}
}

func TestSetOfFTSInputsBecomesDocKeys(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{"as": "ale", "verb": "search", "strategy": "fts", "query_text": "ale"},
			map[string]any{
				"as":      "both",
				"verb":    "set",
				"op":      "union",
				"id_kind": "node_ids",
				"inputs":  []any{"@peach.ids", "@ale.ids"},
			},
			map[string]any{"as": "rows", "verb": "project", "ids": "@both.ids"},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if !reflect.DeepEqual(changed, []string{"peach", "ale", "both"}) {
		t.Fatalf("changed %v", changed)
	}
	steps := out["steps"].([]any)
	set := steps[2].(map[string]any)
	if !reflect.DeepEqual(set["inputs"], []any{"@peach.doc_keys", "@ale.doc_keys"}) || set["id_kind"] != "doc_keys" {
		t.Fatalf("set %v", set)
	}
	if steps[3].(map[string]any)["ids"] != "@both.doc_keys" {
		t.Fatalf("project %v", steps[3])
	}
}

func TestSetDocKeysRewritesFTSInputs(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{
				"as":   "both",
				"verb": "set",
				"params": map[string]any{
					"op":      "union",
					"id_kind": "doc_keys",
					"inputs":  []any{"@peach.ids", "@peach.doc_keys"},
				},
			},
			map[string]any{"as": "rows", "verb": "project", "ids": "@both.ids"},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if !reflect.DeepEqual(changed, []string{"peach", "both"}) {
		t.Fatalf("changed %v", changed)
	}
	params := out["steps"].([]any)[1].(map[string]any)["params"].(map[string]any)
	if !reflect.DeepEqual(params["inputs"], []any{"@peach.doc_keys", "@peach.doc_keys"}) || params["id_kind"] != "doc_keys" {
		t.Fatalf("params %v", params)
	}
	if out["steps"].([]any)[2].(map[string]any)["ids"] != "@both.doc_keys" {
		t.Fatalf("project %v", out["steps"].([]any)[2])
	}
}

func TestToolNameAndArgsCountAsFTS(t *testing.T) {
	shapes := []map[string]any{
		{"name": "peach", "tool": "search", "args": map[string]any{"strategy": "fts", "query_text": "peach"}},
		{"as": "peach", "verb": "search", "args": map[string]any{"strategy": "fts"}},
		{"name": "peach", "tool": "search", "strategy": "fts", "query_text": "peach"},
	}
	for _, search := range shapes {
		body := map[string]any{
			"steps": []any{
				search,
				map[string]any{"as": "rows", "verb": "project", "ids": "@peach.ids"},
			},
		}
		out, changed := RewriteFTSIdsToDocKeys(body)
		if !reflect.DeepEqual(changed, []string{"peach"}) {
			t.Fatalf("changed %v search %v", changed, search)
		}
		if out["steps"].([]any)[1].(map[string]any)["ids"] != "@peach.doc_keys" {
			t.Fatalf("%v", out["steps"])
		}
	}
}

func TestParamsBagHidesAFlatStrategy(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{
				"as":       "hits",
				"verb":     "search",
				"strategy": "fts",
				"params":   map[string]any{"query_text": "peach"},
			},
			map[string]any{"as": "rows", "verb": "project", "ids": "@hits.ids"},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if len(changed) != 0 {
		t.Fatalf("changed %v", changed)
	}
	if out["steps"].([]any)[1].(map[string]any)["ids"] != "@hits.ids" {
		t.Fatalf("%v", out["steps"])
	}
}

func TestArgsByIDPromotesWhenTheFlatByIsAField(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{
				"as":   "ranked",
				"verb": "order",
				"by":   "field:name",
				"args": map[string]any{"ids": "@peach.ids", "by": "id"},
			},
			map[string]any{"as": "rows", "verb": "project", "ids": "@ranked.ids"},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if !reflect.DeepEqual(changed, []string{"peach", "ranked"}) {
		t.Fatalf("changed %v", changed)
	}
	steps := out["steps"].([]any)
	args := steps[1].(map[string]any)["args"].(map[string]any)
	if args["ids"] != "@peach.doc_keys" || steps[1].(map[string]any)["by"] != "field:name" {
		t.Fatalf("order %v", steps[1])
	}
	if steps[2].(map[string]any)["ids"] != "@ranked.doc_keys" {
		t.Fatalf("project %v", steps[2])
	}
}

func TestCarryIgnoresAMentionOutsideIDs(t *testing.T) {
	body := map[string]any{
		"steps": []any{
			map[string]any{"as": "peach", "verb": "search", "strategy": "fts", "query_text": "peach"},
			map[string]any{
				"as":   "ranked",
				"verb": "order",
				"ids":  "@other.ids",
				"by":   "id",
				"note": "see @peach.doc_keys",
			},
			map[string]any{"as": "rows", "verb": "project", "ids": "@ranked.ids"},
		},
	}
	out, changed := RewriteFTSIdsToDocKeys(body)
	if len(changed) != 0 {
		t.Fatalf("changed %v", changed)
	}
	steps := out["steps"].([]any)
	if steps[1].(map[string]any)["note"] != "see @peach.doc_keys" {
		t.Fatalf("note %v", steps[1])
	}
	if steps[2].(map[string]any)["ids"] != "@ranked.ids" {
		t.Fatalf("project %v", steps[2])
	}
}
