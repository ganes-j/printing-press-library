// Package food52 contains Food52-specific helpers used by the printed CLI:
// __NEXT_DATA__ extraction, typed Recipe/Article wrappers, runtime discovery
// of the Next.js buildId and the public Typesense search-only key, and the
// curated recipe-tag enum.
//
// All commands in internal/cli/ that talk to food52.com or to the Typesense
// search endpoint route through this package — the printed CLI's data layer
// lives here, not in the auto-generated client.
package food52

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// nextDataRe matches the Next.js SSR data <script> block on every Food52 page.
// The (?s) flag lets the body span lines.
var nextDataRe = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)

// ExtractNextData pulls the parsed __NEXT_DATA__ JSON object out of a Food52
// HTML page. It returns ErrNoNextData when the script block is absent — that
// is the signal a page failed to render (challenge HTML, 404, or a route the
// CLI does not yet understand) rather than a transient error.
func ExtractNextData(html []byte) (map[string]any, error) {
	m := nextDataRe.FindSubmatch(html)
	if len(m) < 2 {
		return nil, ErrNoNextData
	}
	var out map[string]any
	if err := json.Unmarshal(m[1], &out); err != nil {
		return nil, fmt.Errorf("food52: parsing __NEXT_DATA__: %w", err)
	}
	return out, nil
}

// PageProps returns the inner pageProps map. Every Food52 SSR page has this
// shape (props.pageProps); the field that hangs off it differs per route
// (recipe, recipesByTag, blogPost, blogPosts, ...).
func PageProps(nd map[string]any) (map[string]any, error) {
	props, ok := nd["props"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("food52: __NEXT_DATA__.props missing")
	}
	pp, ok := props["pageProps"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("food52: __NEXT_DATA__.props.pageProps missing")
	}
	return pp, nil
}

// BuildID returns the Next.js build identifier from a parsed __NEXT_DATA__.
// The buildId rotates on every Food52 deploy; commands that hit the
// /_next/data/<buildId>/<path>.json endpoint must refresh it from a fresh
// page fetch when a 404 comes back.
func BuildID(nd map[string]any) string {
	if v, ok := nd["buildId"].(string); ok {
		return v
	}
	return ""
}

// LooksLikeChallenge returns true when an HTML body matches Vercel's bot
// mitigation page. The CLI's transport is supposed to clear this — when it
// shows up here it almost always means the user's printing-press binary was
// built without Surf or that Vercel rotated its fingerprint heuristics.
func LooksLikeChallenge(html []byte) bool {
	s := strings.ToLower(string(html))
	return strings.Contains(s, "vercel security checkpoint") ||
		strings.Contains(s, "x-vercel-challenge") ||
		strings.Contains(s, "verifying you are human")
}

// ErrNoNextData is returned when an HTML page has no __NEXT_DATA__ block.
var ErrNoNextData = fmt.Errorf("food52: no __NEXT_DATA__ block in page")
