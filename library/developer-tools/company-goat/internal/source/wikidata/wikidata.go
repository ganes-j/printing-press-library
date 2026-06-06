// Package wikidata wraps the Wikidata SPARQL endpoint at
// https://query.wikidata.org/sparql. Used for company reference facts
// (founded date, founders, HQ, industry).
package wikidata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	HTTP      *http.Client
	UserAgent string
}

func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 20 * time.Second},
		UserAgent: "company-goat-pp-cli/0.1 (https://github.com/mvanhorn/printing-press-library)",
	}
}

// Company is the Wikidata-derived shape we return.
type Company struct {
	QID         string   `json:"qid"`         // wikidata Q-identifier
	Label       string   `json:"label"`       // English label
	Description string   `json:"description"` // English description
	Website     string   `json:"website,omitempty"`
	Founded     string   `json:"founded,omitempty"`     // ISO date
	HQLocation  string   `json:"hq_location,omitempty"` // English label of headquarters location
	Country     string   `json:"country,omitempty"`
	Industry    string   `json:"industry,omitempty"`
	Founders    []string `json:"founders,omitempty"`
}

// LookupByDomain looks up a company in Wikidata via its official website
// (P856). Returns nil, nil if no match. domain should be the bare host
// (e.g. "stripe.com").
func (c *Client) LookupByDomain(ctx context.Context, domain string) (*Company, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "www.")
	if domain == "" {
		return nil, errors.New("empty domain")
	}

	// Wikidata stores P856 URLs canonically — most use https:// + bare domain.
	// We try both with and without www. and both http/https.
	values := []string{
		"<https://" + domain + ">",
		"<https://www." + domain + ">",
		"<http://" + domain + ">",
	}

	q := fmt.Sprintf(`
SELECT ?company ?companyLabel ?companyDescription ?website ?founded ?hqLabel ?countryLabel ?industryLabel
       (GROUP_CONCAT(DISTINCT ?founderLabel; separator=", ") AS ?founders)
WHERE {
  VALUES ?website { %s }
  ?company wdt:P856 ?website .
  OPTIONAL { ?company wdt:P571 ?founded . }
  OPTIONAL { ?company wdt:P159 ?hq . ?hq rdfs:label ?hqLabel . FILTER(LANG(?hqLabel) = "en") }
  OPTIONAL { ?company wdt:P17 ?country . ?country rdfs:label ?countryLabel . FILTER(LANG(?countryLabel) = "en") }
  OPTIONAL { ?company wdt:P452 ?industry . ?industry rdfs:label ?industryLabel . FILTER(LANG(?industryLabel) = "en") }
  OPTIONAL { ?company wdt:P112 ?founder . ?founder rdfs:label ?founderLabel . FILTER(LANG(?founderLabel) = "en") }
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en". }
}
GROUP BY ?company ?companyLabel ?companyDescription ?website ?founded ?hqLabel ?countryLabel ?industryLabel
LIMIT 1`, strings.Join(values, " "))

	body, err := c.runSPARQL(ctx, q)
	if err != nil {
		return nil, err
	}
	results, err := parseSPARQL(body)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	r := results[0]
	out := &Company{
		QID:         strings.TrimPrefix(r["company"], "http://www.wikidata.org/entity/"),
		Label:       r["companyLabel"],
		Description: r["companyDescription"],
		Website:     r["website"],
		Founded:     r["founded"],
		HQLocation:  r["hqLabel"],
		Country:     r["countryLabel"],
		Industry:    r["industryLabel"],
	}
	if r["founders"] != "" {
		for _, f := range strings.Split(r["founders"], ", ") {
			f = strings.TrimSpace(f)
			if f != "" {
				out.Founders = append(out.Founders, f)
			}
		}
	}
	return out, nil
}

// SearchByName uses the Wikidata wbsearchentities API (REST) to find
// candidate companies by name. Faster than SPARQL when we just need a
// candidate list for resolution.
func (c *Client) SearchByName(ctx context.Context, query string, limit int) ([]CandidateMatch, error) {
	if query == "" {
		return nil, errors.New("empty query")
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	u, _ := url.Parse("https://www.wikidata.org/w/api.php")
	q := u.Query()
	q.Set("action", "wbsearchentities")
	q.Set("search", query)
	q.Set("language", "en")
	q.Set("format", "json")
	q.Set("type", "item")
	q.Set("limit", fmt.Sprintf("%d", limit))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("wikidata search %d: %s", resp.StatusCode, string(body))
	}
	var raw struct {
		Search []struct {
			ID          string `json:"id"`
			Label       string `json:"label"`
			Description string `json:"description"`
			ConceptURI  string `json:"concepturi"`
		} `json:"search"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]CandidateMatch, 0, len(raw.Search))
	for _, r := range raw.Search {
		out = append(out, CandidateMatch{
			QID:         r.ID,
			Label:       r.Label,
			Description: r.Description,
			ConceptURI:  r.ConceptURI,
		})
	}
	return out, nil
}

// CandidateMatch is one result from SearchByName, intended for the
// resolver's disambiguation list.
type CandidateMatch struct {
	QID         string
	Label       string
	Description string
	ConceptURI  string
}

// LookupWebsiteForQID returns the official website (P856) for a Wikidata
// entity. Used during name resolution to map a Wikidata candidate to a
// canonical domain.
func (c *Client) LookupWebsiteForQID(ctx context.Context, qid string) (string, error) {
	q := fmt.Sprintf(`
SELECT ?website WHERE {
  wd:%s wdt:P856 ?website .
} LIMIT 1`, qid)
	body, err := c.runSPARQL(ctx, q)
	if err != nil {
		return "", err
	}
	results, err := parseSPARQL(body)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", nil
	}
	return results[0]["website"], nil
}

func (c *Client) runSPARQL(ctx context.Context, query string) ([]byte, error) {
	u, _ := url.Parse("https://query.wikidata.org/sparql")
	q := u.Query()
	q.Set("query", query)
	q.Set("format", "json")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/sparql-results+json")
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
		return nil, fmt.Errorf("wikidata sparql %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func parseSPARQL(body []byte) ([]map[string]string, error) {
	var raw struct {
		Results struct {
			Bindings []map[string]struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode sparql: %w", err)
	}
	out := make([]map[string]string, 0, len(raw.Results.Bindings))
	for _, b := range raw.Results.Bindings {
		flat := make(map[string]string, len(b))
		for k, v := range b {
			flat[k] = v.Value
		}
		out = append(out, flat)
	}
	return out, nil
}
