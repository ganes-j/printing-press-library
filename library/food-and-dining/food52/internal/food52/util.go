package food52

import (
	"encoding/json"
	"strings"
)

// stringField returns m[key] as a string, "" otherwise. Tolerates nil maps.
func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func boolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, _ := m[key].(bool)
	return v
}

func floatField(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}

func intField(m map[string]any, key string) int {
	return int(floatField(m, key))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func stringSlice(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if strings.TrimSpace(single) == "" {
			return nil
		}
		return []string{single}
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	return nil
}

func splitKeywords(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func jsonStringEq(raw json.RawMessage, want string) bool {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s == want
	}
	return false
}

// indexFold is a case-insensitive substring search.
func indexFold(s, substr string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(substr))
}

// indexAny returns the first index of any byte in chars, or -1.
func indexAny(s, chars string) int {
	return strings.IndexAny(s, chars)
}

// tagNames extracts a flat slice of tag display names from the SSR `tags`
// field, which may be a list of strings, a list of {name,_id}, or a Sanity
// reference list.
func tagNames(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, item := range arr {
		var name string
		switch t := item.(type) {
		case string:
			name = t
		case map[string]any:
			name = firstNonEmpty(stringField(t, "name"), stringField(t, "title"), stringField(t, "slug"))
		}
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// extractImageURL pulls a usable URL out of the SSR featuredImage object,
// which may carry asset.url, externalUrl, or a nested src.
func extractImageURL(v any) string {
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if u := stringField(obj, "externalUrl"); u != "" {
		return u
	}
	if asset, ok := obj["asset"].(map[string]any); ok {
		if u := stringField(asset, "url"); u != "" {
			return u
		}
		if ref := stringField(asset, "_ref"); strings.HasPrefix(ref, "image-") {
			return sanityRefToURL(ref)
		}
	}
	if src := stringField(obj, "src"); src != "" {
		return src
	}
	if u := stringField(obj, "url"); u != "" {
		return u
	}
	return ""
}

// sanityRefToURL converts a Sanity image-<id>-<dims>-<fmt> reference into a
// CDN URL using Food52's known project + dataset.
//
// Reference: image-c6388d60caf557e5cb0365f5b2f56634e3bf8e46-698x926-png
//
//	→ https://cdn.sanity.io/images/7ea75ra6/production/c6388d60caf557e5cb0365f5b2f56634e3bf8e46-698x926.png
func sanityRefToURL(ref string) string {
	rest := strings.TrimPrefix(ref, "image-")
	parts := strings.Split(rest, "-")
	if len(parts) < 3 {
		return ""
	}
	id := parts[0]
	dims := parts[1]
	ext := parts[len(parts)-1]
	return "https://cdn.sanity.io/images/7ea75ra6/production/" + id + "-" + dims + "." + ext
}

// flattenSanityBlocks walks a Sanity portable-text array and concatenates the
// rendered text. Sanity blocks look like:
//
//	[{"_type":"block","children":[{"_type":"span","text":"Hello "},{"_type":"span","text":"world"}]},
//	 {"_type":"block","children":[...]}]
//
// We do not preserve marks (bold, italic) — the printed CLI consumers want
// readable text, not styled HTML.
func flattenSanityBlocks(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		parts := []string{}
		for _, item := range t {
			parts = append(parts, flattenSanityBlocks(item))
		}
		return strings.TrimSpace(strings.Join(parts, "\n\n"))
	case map[string]any:
		typeStr := stringField(t, "_type")
		switch typeStr {
		case "block":
			children, _ := t["children"].([]any)
			var b strings.Builder
			for _, c := range children {
				if cm, ok := c.(map[string]any); ok {
					if txt, ok := cm["text"].(string); ok {
						b.WriteString(txt)
					}
				}
			}
			return strings.TrimSpace(b.String())
		case "span":
			return stringField(t, "text")
		default:
			// Some content types (image, code, divider, embed) don't carry text.
			// Skip silently rather than emit JSON noise.
			return ""
		}
	}
	return ""
}

// cleanIngredientStrings strips literal " undefined " tokens (caused by
// Food52's CMS rendering missing qty/unit fields with the JS literal
// "undefined") and collapses extra whitespace. Returns the slice in place
// so callers don't have to allocate twice.
func cleanIngredientStrings(in []string) []string {
	for i, s := range in {
		// Replace standalone undefined tokens (with surrounding spaces) and
		// leading/trailing undefined tokens.
		s = strings.ReplaceAll(s, " undefined ", " ")
		s = strings.TrimPrefix(s, "undefined ")
		s = strings.TrimSuffix(s, " undefined")
		// Collapse runs of internal whitespace to single spaces.
		for strings.Contains(s, "  ") {
			s = strings.ReplaceAll(s, "  ", " ")
		}
		in[i] = strings.TrimSpace(s)
	}
	return in
}

// sanityIngredientLines extracts ingredient strings from the SSR
// recipeDetails.ingredients structure. Each ingredient is typically
// {"_type":"ingredient","quantity":"2","unit":"cups","name":"flour", ...} or
// {"_type":"ingredientText","text":"Salt to taste"}.
func sanityIngredientLines(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typeStr := stringField(obj, "_type")
		if typeStr == "ingredientText" || typeStr == "block" {
			if txt := flattenSanityBlocks(item); txt != "" {
				out = append(out, txt)
			}
			continue
		}
		// "ingredient" or unspecified — assemble qty + unit + name + note.
		// Sanity often serializes a missing unit as the literal string
		// "undefined" because the JS-side field is a typed enum that
		// renders unset values that way. Skip those.
		parts := []string{}
		if q := stringField(obj, "quantity"); q != "" && q != "undefined" {
			parts = append(parts, q)
		}
		if u := stringField(obj, "unit"); u != "" && u != "undefined" {
			parts = append(parts, u)
		}
		if n := stringField(obj, "name"); n != "" && n != "undefined" {
			parts = append(parts, n)
		}
		if note := stringField(obj, "note"); note != "" {
			parts = append(parts, "("+note+")")
		}
		if len(parts) > 0 {
			out = append(out, strings.Join(parts, " "))
		}
	}
	return out
}

// sanityInstructionLines pulls numbered step strings from the SSR
// recipeDetails.instructions, which is typically a list of blocks (one per
// step) or a list of {step, body} records.
func sanityInstructionLines(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// Direct text field
		if txt := stringField(obj, "text"); txt != "" {
			out = append(out, txt)
			continue
		}
		// Body containing portable text
		if body, ok := obj["body"]; ok {
			if t := flattenSanityBlocks(body); t != "" {
				out = append(out, t)
				continue
			}
		}
		// Or the item itself is a block
		if t := flattenSanityBlocks(item); t != "" {
			out = append(out, t)
		}
	}
	return out
}
