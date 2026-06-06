// Package resolve handles "name → canonical domain" lookups across
// Wikidata, YC directory, and direct DNS probes. When a name is
// ambiguous, callers receive a candidate list to render for
// disambiguation.
package resolve

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/company-goat/internal/source/wikidata"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/company-goat/internal/source/yc"
)

// Candidate is one possible match for a name lookup.
type Candidate struct {
	Domain      string `json:"domain"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`         // "wikidata", "yc", "domain-probe"
	Year        string `json:"year,omitempty"` // founding year if known, helps disambiguate
}

// Resolution is the outcome of a name lookup.
type Resolution struct {
	// AutoResolved is set when exactly one high-confidence candidate
	// emerged. Domain holds the canonical form.
	AutoResolved bool   `json:"auto_resolved"`
	Domain       string `json:"domain,omitempty"`
	Source       string `json:"source,omitempty"`
	// Candidates is populated when multiple matches exist. Empty when
	// AutoResolved or when no matches found at all.
	Candidates []Candidate `json:"candidates,omitempty"`
	// NoCandidates is set when nothing matched. The caller should print
	// a "try --domain" hint.
	NoCandidates bool `json:"no_candidates"`
	// Query echoes the input.
	Query string `json:"query"`
}

// Resolver runs the multi-source resolution.
type Resolver struct {
	WD *wikidata.Client
	YC *yc.Client
}

func NewResolver() *Resolver {
	return &Resolver{
		WD: wikidata.NewClient(),
		YC: yc.NewClient(),
	}
}

// Resolve takes a name (or already-a-domain, in which case it short-circuits)
// and returns either an auto-resolved domain or a candidate list.
func (r *Resolver) Resolve(ctx context.Context, query string) (*Resolution, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, errors.New("empty query")
	}

	// If the input already looks like a domain, accept it as-is.
	if looksLikeDomain(q) {
		return &Resolution{
			AutoResolved: true,
			Domain:       normalizeDomain(q),
			Source:       "user-supplied",
			Query:        query,
		}, nil
	}

	// Run candidate generation in parallel: Wikidata search, YC name match,
	// and DNS probes against common TLDs.
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		all []Candidate
	)

	add := func(c Candidate) {
		mu.Lock()
		defer mu.Unlock()
		all = append(all, c)
	}

	// Wikidata.
	wg.Add(1)
	go func() {
		defer wg.Done()
		wctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		matches, err := r.WD.SearchByName(wctx, q, 5)
		if err != nil || len(matches) == 0 {
			return
		}
		for _, m := range matches {
			ws, _ := r.WD.LookupWebsiteForQID(wctx, m.QID)
			if ws == "" {
				continue
			}
			d := normalizeDomain(ws)
			if d == "" {
				continue
			}
			add(Candidate{
				Domain:      d,
				DisplayName: m.Label,
				Description: m.Description,
				Source:      "wikidata",
			})
		}
	}()

	// YC.
	wg.Add(1)
	go func() {
		defer wg.Done()
		yctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		matches, err := r.YC.FindByName(yctx, q, 5)
		if err != nil || len(matches) == 0 {
			return
		}
		for _, m := range matches {
			d := normalizeDomain(m.Website)
			if d == "" {
				continue
			}
			year := ""
			if m.Batch != "" {
				year = m.Batch
			}
			add(Candidate{
				Domain:      d,
				DisplayName: m.Name,
				Description: m.OneLiner,
				Source:      "yc",
				Year:        year,
			})
		}
	}()

	// Domain probe — try the slug at common TLDs.
	wg.Add(1)
	go func() {
		defer wg.Done()
		slug := strings.ToLower(strings.ReplaceAll(q, " ", ""))
		slug = strings.ReplaceAll(slug, "-", "")
		if slug == "" {
			return
		}
		for _, tld := range []string{"com", "io", "co", "ai", "app"} {
			candidate := slug + "." + tld
			ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
			ips, err := net.DefaultResolver.LookupHost(ctx2, candidate)
			cancel()
			if err == nil && len(ips) > 0 {
				add(Candidate{
					Domain:      candidate,
					DisplayName: q,
					Description: fmt.Sprintf("Probe: %s.%s resolves", slug, tld),
					Source:      "domain-probe",
				})
			}
		}
	}()

	wg.Wait()

	// Dedupe by domain, preferring wikidata > yc > domain-probe.
	deduped := dedupe(all)

	res := &Resolution{Query: query}

	// One high-confidence match (wikidata or yc with reachable domain) → auto-proceed.
	if len(deduped) == 1 {
		res.AutoResolved = true
		res.Domain = deduped[0].Domain
		res.Source = deduped[0].Source
		return res, nil
	}

	// Multiple matches → return candidate list.
	if len(deduped) > 1 {
		res.Candidates = deduped
		return res, nil
	}

	// Zero matches.
	res.NoCandidates = true
	return res, nil
}

func dedupe(in []Candidate) []Candidate {
	priority := map[string]int{"wikidata": 0, "yc": 1, "domain-probe": 2, "user-supplied": -1}
	seen := map[string]int{} // domain -> idx in out
	var out []Candidate
	for _, c := range in {
		if c.Domain == "" {
			continue
		}
		c.Domain = normalizeDomain(c.Domain)
		if c.Domain == "" {
			continue
		}
		idx, exists := seen[c.Domain]
		if !exists {
			seen[c.Domain] = len(out)
			out = append(out, c)
			continue
		}
		// Prefer the higher-priority source.
		if priority[c.Source] < priority[out[idx].Source] {
			out[idx] = c
		}
	}
	return out
}

// normalizeDomain strips scheme, www, trailing slashes, and trailing path
// from a URL or domain string.
func normalizeDomain(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(s, prefix) {
			s = s[len(prefix):]
		}
	}
	s = strings.TrimPrefix(s, "www.")
	if idx := strings.Index(s, "/"); idx > 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "?"); idx > 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "#"); idx > 0 {
		s = s[:idx]
	}
	return strings.TrimRight(s, ".")
}

func looksLikeDomain(s string) bool {
	s = strings.ToLower(s)
	// Must have a dot, no spaces, and the last segment looks TLD-shaped.
	if strings.ContainsAny(s, " \t") {
		return false
	}
	if !strings.Contains(s, ".") {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	last := parts[len(parts)-1]
	if len(last) < 2 || len(last) > 24 {
		return false
	}
	for _, r := range last {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
