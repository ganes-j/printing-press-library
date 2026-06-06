// Package algolia is a small typed wrapper around the Hacker News
// Algolia search API (https://hn.algolia.com/api/v1). It is hand-built
// because the generator's spec format supports a single base_url and
// our primary base_url is the Firebase API; Algolia lives on a separate
// host. Keeping the helper isolated in its own package avoids name
// collisions with generated client code.
package algolia

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://hn.algolia.com/api/v1"

// Client wraps the Algolia API. Construct via New; the zero value is not
// safe to use because it lacks an http.Client.
type Client struct {
	httpc *http.Client
}

// New returns an Algolia client with the given timeout. A timeout <= 0
// falls back to 30 seconds — the API is normally fast but the items
// endpoint walks comment trees and can be slow on large threads.
func New(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{httpc: &http.Client{Timeout: timeout}}
}

// SearchHit is one row from /search or /search_by_date.
type SearchHit struct {
	ObjectID    string   `json:"objectID"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Author      string   `json:"author"`
	Points      int      `json:"points"`
	NumComments int      `json:"num_comments"`
	CreatedAt   string   `json:"created_at"`
	CreatedAtI  int64    `json:"created_at_i"`
	StoryText   string   `json:"story_text"`
	CommentText string   `json:"comment_text"`
	StoryID     int      `json:"story_id"`
	StoryTitle  string   `json:"story_title"`
	StoryURL    string   `json:"story_url"`
	ParentID    int      `json:"parent_id"`
	Tags        []string `json:"_tags"`
}

// SearchResponse is the envelope from /search and /search_by_date.
type SearchResponse struct {
	Hits             []SearchHit `json:"hits"`
	NbHits           int         `json:"nbHits"`
	Page             int         `json:"page"`
	NbPages          int         `json:"nbPages"`
	HitsPerPage      int         `json:"hitsPerPage"`
	ProcessingTimeMS int         `json:"processingTimeMS"`
	Query            string      `json:"query"`
}

// SearchOpts narrows what the agent or user wants. All fields are optional;
// the zero value runs an empty-query relevance search.
type SearchOpts struct {
	// Tags joins multi-value filters with commas (OR), pairs of values
	// expressing AND should be supplied as parenthesized groups by the
	// caller, e.g. "story,(author_pg,author_dang)".
	Tags string
	// NumericFilters is a comma-separated list of expressions like
	// "created_at_i>1700000000,points>10".
	NumericFilters string
	// HitsPerPage caps results per page; Algolia maxes at 1000 but the
	// API rejects > 1000.
	HitsPerPage int
	// Page is zero-indexed.
	Page int
	// ByDate flips to /search_by_date (sort by createdAt desc).
	ByDate bool
}

// Search runs an Algolia query and returns the parsed envelope. Empty
// query strings are valid — Algolia interprets "all matches" within the
// supplied tag/numeric filters.
func (c *Client) Search(query string, opts SearchOpts) (*SearchResponse, error) {
	endpoint := "/search"
	if opts.ByDate {
		endpoint = "/search_by_date"
	}
	q := url.Values{}
	q.Set("query", query)
	if opts.Tags != "" {
		q.Set("tags", opts.Tags)
	}
	if opts.NumericFilters != "" {
		q.Set("numericFilters", opts.NumericFilters)
	}
	if opts.HitsPerPage > 0 {
		q.Set("hitsPerPage", fmt.Sprintf("%d", opts.HitsPerPage))
	}
	if opts.Page > 0 {
		q.Set("page", fmt.Sprintf("%d", opts.Page))
	}
	target := baseURL + endpoint + "?" + q.Encode()

	resp, err := c.httpc.Get(target)
	if err != nil {
		return nil, fmt.Errorf("algolia GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading algolia response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("algolia returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out SearchResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parsing algolia response: %w", err)
	}
	return &out, nil
}

// ItemNode is one node in the Algolia /items/{id} comment tree.
// Children is recursive — leaves have an empty Children slice.
type ItemNode struct {
	ID         int        `json:"id"`
	CreatedAt  string     `json:"created_at"`
	CreatedAtI int64      `json:"created_at_i"`
	Type       string     `json:"type"`
	Author     string     `json:"author"`
	Title      string     `json:"title,omitempty"`
	URL        string     `json:"url,omitempty"`
	Text       string     `json:"text"`
	Points     int        `json:"points"`
	ParentID   int        `json:"parent_id,omitempty"`
	StoryID    int        `json:"story_id,omitempty"`
	Children   []ItemNode `json:"children"`
}

// Item fetches a full item with its comment tree from Algolia. Returns
// the root node; walk Children for the tree.
func (c *Client) Item(id string) (*ItemNode, error) {
	target := baseURL + "/items/" + url.PathEscape(id)
	resp, err := c.httpc.Get(target)
	if err != nil {
		return nil, fmt.Errorf("algolia item GET %s: %w", id, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading item response: %w", err)
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("item %s not found", id)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("algolia item returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out ItemNode
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parsing item response: %w", err)
	}
	return &out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
