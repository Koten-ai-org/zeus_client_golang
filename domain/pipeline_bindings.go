// SPDX-License-Identifier: BUSL-1.1

package domain

import (
	"regexp"
	"sort"
	"strings"
)

// rewriteVerbs accept either a node-id list or a doc-key list.
var rewriteVerbs = map[string]struct{}{
	"project": {},
	"order":   {},
	"enrich":  {},
}

var (
	orderIdent = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	bindingRef = regexp.MustCompile(`@([^\s@.]+)\.(ids|doc_keys|scored|node_ids|entity_ids|edge_ids)\b`)
)

var carryKinds = map[string]struct{}{
	"ids":      {},
	"doc_keys": {},
	"scored":   {},
}

// RewriteFTSIdsToDocKeys returns a pipeline body whose later steps bind FTS
// hits as doc_keys. @step.ids is node ids only. An fts search leaves that list
// empty, so a later project of @step.ids returns no rows.
//
// order by id copies source-key tokens onto doc_keys and leaves node_ids
// empty. A field or created_at ranker fetches and fills both lists, so
// @ranked.ids stays a node-id bind. set copies onto the list named by
// id_kind, so only id_kind doc_keys carries those keys. The rewrite follows
// the keys through project, order, and enrich. get, traverse, and narrow_to
// reject doc keys and keep the node-id binding.
//
// Hybrid, vector, and find steps are left alone. @step.node_ids, an existing
// @step.doc_keys binding, and narrow_to are left alone. The summary is not
// rewritten. The returned names are the steps whose @step.ids token was
// rewritten, in encounter order. When nothing changes, body is returned as
// given.
func RewriteFTSIdsToDocKeys(body map[string]any) (map[string]any, []string) {
	if body == nil {
		return map[string]any{}, nil
	}
	rawSteps, ok := body["steps"].([]any)
	if !ok {
		return body, nil
	}
	producers := map[string]struct{}{}
	for _, name := range ftsStepNames(rawSteps) {
		producers[name] = struct{}{}
	}
	if len(producers) == 0 {
		return body, nil
	}

	var changed []string
	seen := map[string]struct{}{}
	mutated := false
	newSteps := make([]any, 0, len(rawSteps))
	for _, step := range rawSteps {
		sm, isMap := step.(map[string]any)
		if !isMap {
			newSteps = append(newSteps, step)
			continue
		}
		verb := verbOf(sm)
		name := nameOf(sm)
		pattern := producerPattern(producers)
		rewriteStep := false
		flipKind := false
		if pattern != nil {
			if _, ok := rewriteVerbs[verb]; ok {
				rewriteStep = true
			} else if verb == "set" {
				kind := strings.ToLower(strings.TrimSpace(bindingString(paramOf(sm, "id_kind"))))
				if kind == "doc_keys" {
					rewriteStep = true
				} else if inputsAreDocKeys(paramOf(sm, "inputs"), producers) {
					rewriteStep = true
					flipKind = true
				}
			}
		}
		rewritten := sm
		if rewriteStep && pattern != nil {
			before := len(changed)
			rewritten, _ = rewriteValue(cloneMap(sm), pattern, &changed, seen).(map[string]any)
			if flipKind {
				assignParam(rewritten, "id_kind", "doc_keys")
			}
			if len(changed) != before || flipKind {
				mutated = true
			}
		}
		newSteps = append(newSteps, rewritten)
		if name == "" {
			continue
		}
		if verb == "order" && normalizeOrderBy(paramOf(rewritten, "by")) == "id" {
			if hasCarry(paramOf(rewritten, "ids"), producers) {
				producers[name] = struct{}{}
			}
		} else if verb == "set" {
			kind := strings.ToLower(strings.TrimSpace(bindingString(paramOf(rewritten, "id_kind"))))
			if kind == "doc_keys" && hasCarry(paramOf(rewritten, "inputs"), producers) {
				producers[name] = struct{}{}
			}
		}
	}
	if !mutated {
		return body, nil
	}
	out := cloneMap(body)
	out["steps"] = newSteps
	return out, changed
}

func verbOf(step map[string]any) string {
	verb := strings.TrimSpace(bindingString(step["verb"]))
	if verb == "" {
		verb = strings.TrimSpace(bindingString(step["tool"]))
	}
	return strings.ToLower(verb)
}

func nameOf(step map[string]any) string {
	name := strings.TrimSpace(bindingString(step["as"]))
	if name == "" {
		name = strings.TrimSpace(bindingString(step["name"]))
	}
	return name
}

// paramOf is the value Zeus reads. A present params bag wins, then args,
// then a flat key.
func paramOf(step map[string]any, key string) any {
	if params, ok := step["params"].(map[string]any); ok {
		return params[key]
	}
	if args, ok := step["args"].(map[string]any); ok {
		return args[key]
	}
	return step[key]
}

func strategyOf(step map[string]any) string {
	return strings.ToLower(strings.TrimSpace(bindingString(paramOf(step, "strategy"))))
}

func normalizeOrderBy(raw any) string {
	by := strings.TrimSpace(bindingString(raw))
	if by == "" || by == "id" || by == "created_at" || strings.HasPrefix(by, "field:") {
		return by
	}
	if orderIdent.MatchString(by) {
		return "field:" + by
	}
	return by
}

func ftsStepNames(steps []any) []string {
	var names []string
	seen := map[string]struct{}{}
	for _, step := range steps {
		sm, ok := step.(map[string]any)
		if !ok {
			continue
		}
		if verbOf(sm) != "search" || strategyOf(sm) != "fts" {
			continue
		}
		name := nameOf(sm)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func producerPattern(producers map[string]struct{}) *regexp.Regexp {
	if len(producers) == 0 {
		return nil
	}
	names := make([]string, 0, len(producers))
	for name := range producers {
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})
	var b strings.Builder
	b.WriteString(`@(`)
	for i, name := range names {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(regexp.QuoteMeta(name))
	}
	b.WriteString(`)\.ids\b`)
	return regexp.MustCompile(b.String())
}

func iterStrings(value any) []string {
	var found []string
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case string:
			found = append(found, v)
		case []any:
			for _, item := range v {
				walk(item)
			}
		case map[string]any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(value)
	return found
}

func hasCarry(value any, producers map[string]struct{}) bool {
	for _, text := range iterStrings(value) {
		for _, ref := range bindingRef.FindAllStringSubmatch(text, -1) {
			if _, ok := producers[ref[1]]; ok {
				if _, ok := carryKinds[ref[2]]; ok {
					return true
				}
			}
		}
	}
	return false
}

// inputsAreDocKeys reports whether every inputs string is only a doc-key
// carry from producers.
func inputsAreDocKeys(inputs any, producers map[string]struct{}) bool {
	texts := iterStrings(inputs)
	if len(texts) == 0 {
		return false
	}
	saw := false
	for _, text := range texts {
		refs := bindingRef.FindAllStringSubmatch(text, -1)
		if len(refs) == 0 || strings.TrimSpace(bindingRef.ReplaceAllString(text, "")) != "" {
			return false
		}
		for _, ref := range refs {
			if _, ok := producers[ref[1]]; ok {
				if _, ok := carryKinds[ref[2]]; ok {
					saw = true
					continue
				}
			}
			return false
		}
	}
	return saw
}

func assignParam(step map[string]any, key string, value any) {
	if params, ok := step["params"].(map[string]any); ok {
		params[key] = value
		return
	}
	if args, ok := step["args"].(map[string]any); ok {
		args[key] = value
		return
	}
	step[key] = value
}

func rewriteValue(value any, pattern *regexp.Regexp, changed *[]string, seen map[string]struct{}) any {
	switch v := value.(type) {
	case string:
		return pattern.ReplaceAllStringFunc(v, func(match string) string {
			sub := pattern.FindStringSubmatch(match)
			if len(sub) < 2 {
				return match
			}
			step := sub[1]
			if _, ok := seen[step]; !ok {
				seen[step] = struct{}{}
				*changed = append(*changed, step)
			}
			return "@" + step + ".doc_keys"
		})
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = rewriteValue(item, pattern, changed, seen)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if key == "narrow_to" {
				out[key] = item
				continue
			}
			out[key] = rewriteValue(item, pattern, changed, seen)
		}
		return out
	default:
		return value
	}
}

func bindingString(v any) string {
	s, _ := v.(string)
	return s
}
