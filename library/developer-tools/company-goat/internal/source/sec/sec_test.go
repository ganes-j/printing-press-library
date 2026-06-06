package sec

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSearchAndFetchAllCapsParsedFilings(t *testing.T) {
	c := NewClient("test@example.com")
	c.limiter = nil
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Host, "efts.sec.gov"):
			body := `{
				"hits": {
					"total": {"value": 2},
					"hits": [
						{"_id":"0000000001-26-000001:primary_doc.xml","_source":{"ciks":["0000000001"],"form":"D","file_date":"2026-01-01","adsh":"0000000001-26-000001"}},
						{"_id":"0000000002-26-000001:primary_doc.xml","_source":{"ciks":["0000000002"],"form":"D","file_date":"2026-01-02","adsh":"0000000002-26-000001"}}
					]
				}
			}`
			return response(200, body), nil
		case strings.Contains(req.URL.Path, "000000000126000001"):
			return response(200, formDXML("First Issuer")), nil
		case strings.Contains(req.URL.Path, "000000000226000001"):
			return response(200, formDXML("Second Issuer")), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	})}

	got, err := c.SearchAndFetchAll(context.Background(), "issuer", 1)
	if err != nil {
		t.Fatalf("SearchAndFetchAll returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one parsed filing, got %d: %+v", len(got), got)
	}
	if got[0].EntityName != "First Issuer" {
		t.Fatalf("expected first parsed filing, got %+v", got[0])
	}
}

func TestGetRetries429WithRetryAfter(t *testing.T) {
	var requests int
	var sleeps []time.Duration
	c := NewClient("test@example.com")
	c.limiter = nil
	c.sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			resp := response(429, "slow down")
			resp.Header.Set("Retry-After", "2")
			return resp, nil
		}
		return response(200, `{"ok":true}`), nil
	})}

	body, err := c.get(context.Background(), "https://www.sec.gov/test")
	if err != nil {
		t.Fatalf("get returned error: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests, got %d", requests)
	}
	if len(sleeps) != 1 || sleeps[0] != 2*time.Second {
		t.Fatalf("expected one 2s retry-after sleep, got %+v", sleeps)
	}
}

func TestGetExhausted429ReturnsRateLimitError(t *testing.T) {
	c := NewClient("test@example.com")
	c.limiter = nil
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(429, "too many requests"), nil
	})}

	_, err := c.get(context.Background(), "https://www.sec.gov/test")
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if !strings.Contains(rateErr.Error(), "HTTP 429") {
		t.Fatalf("rate limit error should mention HTTP 429: %v", rateErr)
	}
}

func TestSearchAndFetchAllPropagatesRateLimitFromFilingFetch(t *testing.T) {
	c := NewClient("test@example.com")
	c.limiter = nil
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "efts.sec.gov") {
			body := `{
				"hits": {
					"total": {"value": 1},
					"hits": [
						{"_id":"0000000001-26-000001:primary_doc.xml","_source":{"ciks":["0000000001"],"form":"D","file_date":"2026-01-01","adsh":"0000000001-26-000001"}}
					]
				}
			}`
			return response(200, body), nil
		}
		return response(429, "too many requests"), nil
	})}

	_, err := c.SearchAndFetchAll(context.Background(), "issuer", 1)
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
}

func TestSearchAnyFormOmitsFormsFilter(t *testing.T) {
	var capturedURL string
	c := NewClient("test@example.com")
	c.limiter = nil
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		body := `{
			"hits": {
				"total": {"value": 3},
				"hits": [
					{"_id":"0000000001-21-000001:primary_doc.xml","_source":{"ciks":["0000000001"],"form":"10-K","file_date":"2021-12-14","display_names":["Weber Inc."],"adsh":"0000000001-21-000001"}},
					{"_id":"0000000002-19-000001:primary_doc.xml","_source":{"ciks":["0000000002"],"form":"10-Q","file_date":"2019-08-14","display_names":["Venture Lending & Leasing VII, Inc."],"adsh":"0000000002-19-000001"}},
					{"_id":"0000000003-21-000001:primary_doc.xml","_source":{"ciks":["0000000003"],"form":"8-K","file_date":"2021-11-18","display_names":["Weber Inc."],"adsh":"0000000003-21-000001"}}
				]
			}
		}`
		return response(200, body), nil
	})}

	resp, err := c.SearchAnyForm(context.Background(), "June Life Inc", 25)
	if err != nil {
		t.Fatalf("SearchAnyForm returned error: %v", err)
	}
	if strings.Contains(capturedURL, "forms=") {
		t.Fatalf("expected no forms= filter in URL, got %s", capturedURL)
	}
	if !strings.Contains(capturedURL, "q=%22June+Life+Inc%22") {
		t.Fatalf("expected quoted query in URL, got %s", capturedURL)
	}
	if resp.Total != 3 {
		t.Fatalf("expected total=3, got %d", resp.Total)
	}
	if len(resp.Hits) != 3 {
		t.Fatalf("expected 3 hits, got %d", len(resp.Hits))
	}
	gotForms := []string{resp.Hits[0].Form, resp.Hits[1].Form, resp.Hits[2].Form}
	wantForms := []string{"10-K", "10-Q", "8-K"}
	for i, got := range gotForms {
		if got != wantForms[i] {
			t.Fatalf("hit %d form: got %q, want %q", i, got, wantForms[i])
		}
	}
}

func TestSearchAnyFormEmptyResults(t *testing.T) {
	c := NewClient("test@example.com")
	c.limiter = nil
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(200, `{"hits":{"total":{"value":0},"hits":[]}}`), nil
	})}

	resp, err := c.SearchAnyForm(context.Background(), "no-such-issuer-12345", 10)
	if err != nil {
		t.Fatalf("SearchAnyForm returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response, got nil")
	}
	if resp.Total != 0 || len(resp.Hits) != 0 {
		t.Fatalf("expected empty response, got total=%d hits=%d", resp.Total, len(resp.Hits))
	}
}

func TestSearchFormDStillIncludesFormsFilter(t *testing.T) {
	// Refactor parity: SearchFormD must continue passing forms=D after the
	// shared searchEFTS extraction.
	var capturedURL string
	c := NewClient("test@example.com")
	c.limiter = nil
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return response(200, `{"hits":{"total":{"value":0},"hits":[]}}`), nil
	})}

	if _, err := c.SearchFormD(context.Background(), "stripe", 10); err != nil {
		t.Fatalf("SearchFormD returned error: %v", err)
	}
	if !strings.Contains(capturedURL, "forms=D") {
		t.Fatalf("expected forms=D in URL after refactor, got %s", capturedURL)
	}
}

func TestSearchEFTSDecodeFailureReportsContext(t *testing.T) {
	c := NewClient("test@example.com")
	c.limiter = nil
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(200, `{not valid json`), nil
	})}

	_, err := c.SearchAnyForm(context.Background(), "any", 10)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode efts response") {
		t.Fatalf("error should mention 'decode efts response', got: %v", err)
	}
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       ioNopCloser{strings.NewReader(body)},
		Header:     make(http.Header),
	}
}

type ioNopCloser struct {
	*strings.Reader
}

func (c ioNopCloser) Close() error { return nil }

func formDXML(entity string) string {
	return fmt.Sprintf(`<edgarSubmission>
		<primaryIssuer>
			<cik>0000000001</cik>
			<entityName>%s</entityName>
			<entityType>Corporation</entityType>
			<jurisdictionOfInc>DELAWARE</jurisdictionOfInc>
			<yearOfInc><value>2026</value></yearOfInc>
		</primaryIssuer>
		<offeringData>
			<industryGroup><industryGroupType>Technology</industryGroupType></industryGroup>
			<offeringSalesAmounts>
				<totalOfferingAmount>1000000</totalOfferingAmount>
				<totalAmountSold>500000</totalAmountSold>
			</offeringSalesAmounts>
			<federalExemptionsExclusions><item>06B</item></federalExemptionsExclusions>
		</offeringData>
	</edgarSubmission>`, entity)
}
