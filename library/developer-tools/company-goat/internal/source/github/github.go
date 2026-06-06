// Package github wraps the bits of GitHub's REST API used by company-goat-pp-cli:
// org metadata, repo listing, and rate-limit aware client construction.
//
// Auth is optional. Without GITHUB_TOKEN (or `gh auth token`) the unauth
// rate limit is 60/hr; with a token, 5000/hr.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	HTTP  *http.Client
	Token string // empty if unauthenticated
}

// NewClient returns a Client. Resolves a token from GITHUB_TOKEN first,
// then `gh auth token`. Either is fine, both are optional.
func NewClient() *Client {
	c := &Client{HTTP: &http.Client{Timeout: 15 * time.Second}}
	if t := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); t != "" {
		c.Token = t
		return c
	}
	// Try gh auth token, but don't fail if gh isn't installed.
	if _, err := exec.LookPath("gh"); err == nil {
		out, err := exec.Command("gh", "auth", "token").Output()
		if err == nil {
			t := strings.TrimSpace(string(out))
			if t != "" {
				c.Token = t
			}
		}
	}
	return c
}

// IsAuthenticated reports whether a token is present.
func (c *Client) IsAuthenticated() bool { return c.Token != "" }

// Org is the subset of GitHub org fields we use.
type Org struct {
	Login         string `json:"login"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Blog          string `json:"blog"`
	Location      string `json:"location"`
	PublicRepos   int    `json:"public_repos"`
	Followers     int    `json:"followers"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	HTMLURL       string `json:"html_url"`
	TwitterHandle string `json:"twitter_username"`
}

// Repo is the subset of GitHub repo fields used for engineering signals.
type Repo struct {
	Name            string `json:"name"`
	FullName        string `json:"full_name"`
	Description     string `json:"description"`
	Language        string `json:"language"`
	StargazersCount int    `json:"stargazers_count"`
	ForksCount      int    `json:"forks_count"`
	PushedAt        string `json:"pushed_at"`
	UpdatedAt       string `json:"updated_at"`
	CreatedAt       string `json:"created_at"`
	Archived        bool   `json:"archived"`
	Fork            bool   `json:"fork"`
	HTMLURL         string `json:"html_url"`
	Private         bool   `json:"private"`
}

// GetOrg fetches /orgs/{org}. Returns nil, nil if 404.
func (c *Client) GetOrg(ctx context.Context, org string) (*Org, error) {
	if org == "" {
		return nil, errors.New("empty org")
	}
	body, status, err := c.get(ctx, "https://api.github.com/orgs/"+org)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, nil
	}
	if status >= 400 {
		return nil, fmt.Errorf("github %d: %s", status, briefBody(body))
	}
	var o Org
	if err := json.Unmarshal(body, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// ListRepos fetches public repos for an org. up to perPage results.
// Sorts by pushed (most recent activity first).
func (c *Client) ListRepos(ctx context.Context, org string, perPage int) ([]Repo, error) {
	if org == "" {
		return nil, errors.New("empty org")
	}
	if perPage <= 0 || perPage > 100 {
		perPage = 30
	}
	u := fmt.Sprintf("https://api.github.com/orgs/%s/repos?per_page=%d&type=public&sort=pushed", org, perPage)
	body, status, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("github repos %d: %s", status, briefBody(body))
	}
	var repos []Repo
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// FindOrgFromDomain attempts to find a GitHub org whose blog/website
// matches the given domain. Falls back to searching for orgs with that
// name. Returns nil, nil if no match.
//
// This is best-effort. Many companies' GitHub orgs don't list their
// website, and many use a different login than their company name. The
// caller should treat absence as "no GitHub signal" not "no GitHub
// presence."
func (c *Client) FindOrgFromDomain(ctx context.Context, domain, name string) (*Org, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "www.")
	if domain == "" {
		return nil, errors.New("empty domain")
	}

	// First try the obvious heuristic: org name = stem of the domain.
	stem := domain
	if idx := strings.Index(stem, "."); idx > 0 {
		stem = stem[:idx]
	}
	candidates := []string{stem}
	if name != "" {
		simple := strings.ToLower(strings.TrimSpace(name))
		simple = strings.ReplaceAll(simple, " ", "")
		simple = strings.ReplaceAll(simple, "-", "")
		simple = strings.ReplaceAll(simple, ".", "")
		if simple != stem && simple != "" {
			candidates = append(candidates, simple)
		}
		// Also try with hyphen.
		hyphen := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "-")
		if hyphen != stem && hyphen != simple && hyphen != "" {
			candidates = append(candidates, hyphen)
		}
		// And with "s" suffix (e.g. "anthropic" -> "anthropics").
		if !strings.HasSuffix(stem, "s") {
			candidates = append(candidates, stem+"s")
		}
	}

	for _, c2 := range candidates {
		org, err := c.GetOrg(ctx, c2)
		if err != nil {
			return nil, err
		}
		if org == nil {
			continue
		}
		// Verify the blog/url loosely matches the domain — this guards
		// against name collisions ("apollo" the GitHub org vs
		// apollo.io). If blog is set and clearly differs, skip.
		if org.Blog != "" {
			blog := strings.ToLower(org.Blog)
			blog = strings.TrimPrefix(blog, "https://")
			blog = strings.TrimPrefix(blog, "http://")
			blog = strings.TrimPrefix(blog, "www.")
			blog = strings.TrimRight(blog, "/")
			if !strings.HasPrefix(blog, domain) && !strings.HasSuffix(blog, "."+domain) && blog != domain {
				// Ambiguous — return it anyway but caller can warn.
			}
		}
		return org, nil
	}
	return nil, nil
}

// RateLimit reports remaining requests against the Core API.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

// RateLimitStatus probes /rate_limit. Use in doctor to verify auth.
func (c *Client) RateLimitStatus(ctx context.Context) (*RateLimit, error) {
	body, status, err := c.get(ctx, "https://api.github.com/rate_limit")
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("github rate_limit %d", status)
	}
	var raw struct {
		Resources struct {
			Core struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return &RateLimit{
		Limit:     raw.Resources.Core.Limit,
		Remaining: raw.Resources.Core.Remaining,
		Reset:     time.Unix(raw.Resources.Core.Reset, 0),
	}, nil
}

func (c *Client) get(ctx context.Context, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func briefBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// _ silences "unused" warning on strconv during partial development.
var _ = strconv.Itoa
