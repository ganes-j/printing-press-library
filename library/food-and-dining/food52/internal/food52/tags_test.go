package food52

import "testing"

func TestFilterTagsByKind(t *testing.T) {
	tests := []struct {
		kind     string
		minCount int
	}{
		{"", 30},       // all curated tags
		{"meal", 5},    // breakfast, brunch, lunch, dinner, ...
		{"cuisine", 5}, // italian, french, ...
		{"convenience", 3},
		{"unknown-kind", 0},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			got := FilterTagsByKind(tc.kind)
			if len(got) < tc.minCount {
				t.Errorf("kind=%q: got %d tags, want at least %d", tc.kind, len(got), tc.minCount)
			}
		})
	}
}

func TestIsKnownTag(t *testing.T) {
	if !IsKnownTag("chicken") {
		t.Error("expected chicken to be a known tag")
	}
	if IsKnownTag("definitely-not-a-real-tag") {
		t.Error("expected unknown slug to return false")
	}
}

func TestAllTagKinds_NonEmpty(t *testing.T) {
	kinds := AllTagKinds()
	if len(kinds) == 0 {
		t.Error("AllTagKinds should not be empty")
	}
}
