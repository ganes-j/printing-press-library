package food52

import (
	"strings"
	"testing"
)

func TestFindAppBundleURL_Fixture(t *testing.T) {
	html := loadFixture(t, "recipes-chicken.html")
	url, err := findAppBundleURL(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(url, "https://food52.com/_next/static/chunks/pages/_app-") {
		t.Errorf("unexpected bundle URL: %q", url)
	}
	if !strings.HasSuffix(strings.SplitN(url, "?", 2)[0], ".js") {
		t.Errorf("bundle URL doesn't end in .js: %q", url)
	}
}

func TestExtractTypesenseConfig(t *testing.T) {
	bundle := []byte(`...other code... typesense:{host:"foo-1.a1.typesense.net",searchOnlyApiKey:"abc123"}, freestar:...`)
	host, key, err := extractTypesenseConfig(bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "foo-1.a1.typesense.net" {
		t.Errorf("host: got %q, want %q", host, "foo-1.a1.typesense.net")
	}
	if key != "abc123" {
		t.Errorf("key: got %q, want %q", key, "abc123")
	}
}

func TestExtractTypesenseConfig_Missing(t *testing.T) {
	_, _, err := extractTypesenseConfig([]byte("no typesense in here"))
	if err == nil {
		t.Error("expected error when bundle has no typesense block")
	}
}

func TestBuildID_Fixture(t *testing.T) {
	html := loadFixture(t, "recipes-chicken.html")
	nd, err := ExtractNextData(html)
	if err != nil {
		t.Fatalf("ExtractNextData: %v", err)
	}
	id := BuildID(nd)
	if id == "" {
		t.Error("expected a non-empty buildId from Food52 fixture")
	}
}
