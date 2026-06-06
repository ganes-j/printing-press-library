package resolve

import "testing"

func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://stripe.com", "stripe.com"},
		{"http://www.stripe.com/", "stripe.com"},
		{"WWW.STRIPE.COM", "stripe.com"},
		{"https://stripe.com/about", "stripe.com"},
		{"stripe.com?ref=foo", "stripe.com"},
		{"  STRIPE.COM  ", "stripe.com"},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeDomain(c.in)
		if got != c.want {
			t.Errorf("normalizeDomain(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestLooksLikeDomain(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"stripe.com", true},
		{"www.stripe.com", true},
		{"sub.example.io", true},
		{"a.b", false},              // last segment len < 2
		{"a.bb", true},              // last segment len 2
		{"stripe", false},           // no dot
		{"stripe payments", false},  // contains space
		{"stripe.com/about", false}, // last segment contains slash; not a-z0-9
		{"", false},
		{"name-with-spaces in it.com", false},      // contains space
		{"foo.123456789012345678901234567", false}, // last segment too long
	}
	for _, c := range cases {
		got := looksLikeDomain(c.in)
		if got != c.want {
			t.Errorf("looksLikeDomain(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestDedupePrioritizesHigherSource(t *testing.T) {
	in := []Candidate{
		{Domain: "stripe.com", DisplayName: "Stripe", Source: "domain-probe"},
		{Domain: "stripe.com", DisplayName: "Stripe Inc", Source: "wikidata"},
		{Domain: "ramp.com", DisplayName: "Ramp", Source: "yc"},
		{Domain: "ramp.com", DisplayName: "Ramp", Source: "yc"}, // dedup same-source
	}
	got := dedupe(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 deduped entries, got %d: %+v", len(got), got)
	}
	for _, c := range got {
		if c.Domain == "stripe.com" && c.Source != "wikidata" {
			t.Errorf("expected wikidata to win for stripe.com, got %s", c.Source)
		}
	}
}
