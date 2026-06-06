// Package ch wraps the UK Companies House Public Data REST API at
// https://api.company-information.service.gov.uk. Auth is HTTP Basic
// with the API key as username and an empty password (Companies House's
// idiosyncratic Basic-auth shape).
//
// Auth is optional in this CLI. When COMPANIES_HOUSE_API_KEY is unset,
// every call returns ErrNoKey and the calling command prints a one-line
// setup hint instead of an error.
package ch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrNoKey is returned when no Companies House API key is configured.
var ErrNoKey = errors.New("companies house: COMPANIES_HOUSE_API_KEY not set")

type Client struct {
	HTTP *http.Client
	Key  string
}

func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 15 * time.Second},
		Key:  strings.TrimSpace(os.Getenv("COMPANIES_HOUSE_API_KEY")),
	}
}

func (c *Client) HasKey() bool { return c.Key != "" }

// SearchResult is one hit from /search/companies.
type SearchResult struct {
	CompanyNumber   string `json:"company_number"`
	Title           string `json:"title"`
	CompanyStatus   string `json:"company_status"` // active, dissolved, liquidation, etc.
	CompanyType     string `json:"company_type"`   // ltd, plc, llp, etc.
	DateOfCreation  string `json:"date_of_creation"`
	DateOfCessation string `json:"date_of_cessation,omitempty"`
	AddressSnippet  string `json:"address_snippet"`
	Description     string `json:"description"`
	Snippet         string `json:"snippet"`
}

// Search runs /search/companies?q={query}. limit caps the number of
// results returned by Companies House (max 100).
func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if c.Key == "" {
		return nil, ErrNoKey
	}
	if query == "" {
		return nil, errors.New("empty query")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	u, _ := url.Parse("https://api.company-information.service.gov.uk/search/companies")
	q := u.Query()
	q.Set("q", query)
	q.Set("items_per_page", strconv.Itoa(limit))
	u.RawQuery = q.Encode()

	body, status, err := c.get(ctx, u.String())
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("companies house search %d: %s", status, briefBody(body))
	}
	var raw struct {
		Items []SearchResult `json:"items"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw.Items, nil
}

// CompanyProfile is the shape of /company/{number}.
type CompanyProfile struct {
	CompanyNumber           string `json:"company_number"`
	CompanyName             string `json:"company_name"`
	CompanyStatus           string `json:"company_status"`
	CompanyType             string `json:"company_type"`
	DateOfCreation          string `json:"date_of_creation"`
	DateOfCessation         string `json:"date_of_cessation,omitempty"`
	Jurisdiction            string `json:"jurisdiction"`
	RegisteredOfficeAddress struct {
		AddressLine1 string `json:"address_line_1"`
		AddressLine2 string `json:"address_line_2,omitempty"`
		Locality     string `json:"locality"`
		PostalCode   string `json:"postal_code"`
		Country      string `json:"country"`
	} `json:"registered_office_address"`
	SicCodes []string `json:"sic_codes,omitempty"`
}

// GetProfile fetches /company/{number}.
func (c *Client) GetProfile(ctx context.Context, number string) (*CompanyProfile, error) {
	if c.Key == "" {
		return nil, ErrNoKey
	}
	if number == "" {
		return nil, errors.New("empty company number")
	}
	body, status, err := c.get(ctx, "https://api.company-information.service.gov.uk/company/"+number)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, nil
	}
	if status >= 400 {
		return nil, fmt.Errorf("companies house profile %d: %s", status, briefBody(body))
	}
	var p CompanyProfile
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Officer is one item from /company/{number}/officers.
type Officer struct {
	Name        string `json:"name"`
	OfficerRole string `json:"officer_role"`
	AppointedOn string `json:"appointed_on,omitempty"`
	ResignedOn  string `json:"resigned_on,omitempty"`
	Nationality string `json:"nationality,omitempty"`
	Occupation  string `json:"occupation,omitempty"`
}

// ListOfficers fetches /company/{number}/officers.
func (c *Client) ListOfficers(ctx context.Context, number string, limit int) ([]Officer, error) {
	if c.Key == "" {
		return nil, ErrNoKey
	}
	if number == "" {
		return nil, errors.New("empty company number")
	}
	if limit <= 0 || limit > 100 {
		limit = 35
	}
	u := fmt.Sprintf("https://api.company-information.service.gov.uk/company/%s/officers?items_per_page=%d", number, limit)
	body, status, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("companies house officers %d: %s", status, briefBody(body))
	}
	var raw struct {
		Items []Officer `json:"items"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw.Items, nil
}

func (c *Client) get(ctx context.Context, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, 0, err
	}
	// Companies House requires HTTP Basic with key as username, blank password.
	req.SetBasicAuth(c.Key, "")
	req.Header.Set("Accept", "application/json")

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
