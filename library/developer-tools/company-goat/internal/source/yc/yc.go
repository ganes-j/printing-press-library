// Package yc reads the Y Combinator company directory snapshot maintained
// at https://yc-oss.github.io/api/. The snapshot is regenerated daily from
// YC's Algolia index. We fetch a single all-launched JSON and search
// in-process; the file is small enough (~5 MB) that this is cheap.
package yc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Client struct {
	HTTP *http.Client

	mu        sync.Mutex
	companies []Company
	loadedAt  time.Time
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Company is one entry from the YC directory snapshot. Fields are a subset
// of the full schema — only what we surface in commands.
type Company struct {
	Name           string   `json:"name"`
	URL            string   `json:"url"`
	Website        string   `json:"website"`
	Status         string   `json:"status"` // Active, Acquired, Public, Inactive
	Batch          string   `json:"batch"`  // e.g. "S20", "W22"
	OneLiner       string   `json:"one_liner"`
	LongDesc       string   `json:"long_description"`
	Industry       string   `json:"industry"`
	Subindustry    string   `json:"subindustry"`
	Tags           []string `json:"tags"`
	Regions        []string `json:"regions"`
	StageOfCompany string   `json:"stage"`
	TeamSize       int      `json:"team_size"`
	Country        string   `json:"country"`
	LocationCity   string   `json:"location_city"`
}

// LoadAll fetches the all-launched snapshot. Cached in-memory after first
// call (TTL 1h, considered fresh enough since the upstream snapshot is
// daily anyway).
func (c *Client) LoadAll(ctx context.Context) ([]Company, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.companies != nil && time.Since(c.loadedAt) < time.Hour {
		return c.companies, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://yc-oss.github.io/api/companies/all.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		// Fallback path used when /all.json is unavailable.
		return c.fallback(ctx)
	}
	var companies []Company
	if err := json.Unmarshal(body, &companies); err != nil {
		return nil, fmt.Errorf("decode yc snapshot: %w", err)
	}
	c.companies = companies
	c.loadedAt = time.Now()
	return companies, nil
}

func (c *Client) fallback(ctx context.Context) ([]Company, error) {
	for _, path := range []string{"all-launched.json", "companies/all-launched.json"} {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://yc-oss.github.io/api/"+path, nil)
		if err != nil {
			continue
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			var companies []Company
			if err := json.Unmarshal(body, &companies); err == nil {
				c.companies = companies
				c.loadedAt = time.Now()
				return companies, nil
			}
		}
	}
	return nil, errors.New("yc snapshot unreachable from any known path")
}

// FindByName returns up to limit companies whose Name matches query.
// Match is case-insensitive substring. Exact matches are returned first.
func (c *Client) FindByName(ctx context.Context, query string, limit int) ([]Company, error) {
	all, err := c.LoadAll(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, errors.New("empty query")
	}

	var exact, partial []Company
	for _, co := range all {
		nm := strings.ToLower(co.Name)
		switch {
		case nm == q:
			exact = append(exact, co)
		case strings.Contains(nm, q):
			partial = append(partial, co)
		}
	}
	out := append(exact, partial...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// FindByDomain looks up a company by website domain. Returns nil, nil if
// not found.
func (c *Client) FindByDomain(ctx context.Context, domain string) (*Company, error) {
	all, err := c.LoadAll(ctx)
	if err != nil {
		return nil, err
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "www.")
	if domain == "" {
		return nil, nil
	}
	for _, co := range all {
		ws := strings.ToLower(co.Website)
		ws = strings.TrimPrefix(ws, "https://")
		ws = strings.TrimPrefix(ws, "http://")
		ws = strings.TrimPrefix(ws, "www.")
		ws = strings.TrimRight(ws, "/")
		if ws == domain {
			out := co
			return &out, nil
		}
	}
	return nil, nil
}
