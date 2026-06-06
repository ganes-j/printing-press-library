// Package hn wraps the Hacker News Algolia search API at
// https://hn.algolia.com/api/v1/. Used for Show HN posts and mention
// timeline.
package hn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	HTTP *http.Client
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// Hit is one HN story or comment from the Algolia index.
type Hit struct {
	ObjectID    string   `json:"objectID"`
	StoryID     int      `json:"story_id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Author      string   `json:"author"`
	Points      int      `json:"points"`
	NumComments int      `json:"num_comments"`
	CreatedAt   string   `json:"created_at"`
	CreatedAtI  int64    `json:"created_at_i"`
	Tags        []string `json:"_tags"`
}

// SearchResponse is the Algolia search envelope.
type SearchResponse struct {
	Hits         []Hit  `json:"hits"`
	NbHits       int    `json:"nbHits"`
	Page         int    `json:"page"`
	NbPages      int    `json:"nbPages"`
	HitsPerPage  int    `json:"hitsPerPage"`
	ProcessingMS int    `json:"processingTimeMS"`
	Query        string `json:"query"`
}

// SearchShowHN finds Show HN posts mentioning query. Sorted by relevance.
func (c *Client) SearchShowHN(ctx context.Context, query string, hitsPerPage int) (*SearchResponse, error) {
	if hitsPerPage <= 0 {
		hitsPerPage = 20
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("tags", "show_hn")
	q.Set("hitsPerPage", strconv.Itoa(hitsPerPage))
	return c.search(ctx, "search", q)
}

// SearchAll runs a relevance-sorted full-text search over stories matching
// query. Used for mention surfaces.
func (c *Client) SearchAll(ctx context.Context, query string, hitsPerPage int) (*SearchResponse, error) {
	if hitsPerPage <= 0 {
		hitsPerPage = 20
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("tags", "story")
	q.Set("hitsPerPage", strconv.Itoa(hitsPerPage))
	return c.search(ctx, "search", q)
}

// SearchByDate runs a chronological search; useful for building a mention
// timeline. hitsPerPage default 100 (max 1000 by Algolia).
func (c *Client) SearchByDate(ctx context.Context, query string, hitsPerPage int) (*SearchResponse, error) {
	if hitsPerPage <= 0 {
		hitsPerPage = 100
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("tags", "story")
	q.Set("hitsPerPage", strconv.Itoa(hitsPerPage))
	return c.search(ctx, "search_by_date", q)
}

func (c *Client) search(ctx context.Context, endpoint string, q url.Values) (*SearchResponse, error) {
	u := "https://hn.algolia.com/api/v1/" + endpoint + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
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
		return nil, fmt.Errorf("hn algolia %d: %s", resp.StatusCode, briefBody(body))
	}
	var sr SearchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("decode hn response: %w", err)
	}
	return &sr, nil
}

func briefBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
