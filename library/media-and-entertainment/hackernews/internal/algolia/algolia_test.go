package algolia

import "testing"

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"empty", "", 10, ""},
		{"shorter", "hi", 10, "hi"},
		{"exact", "hello", 5, "hello"},
		{"longer", "hello world", 5, "hello..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.in, tc.n)
			if got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

func TestNewClampsTimeout(t *testing.T) {
	c := New(0)
	if c.httpc.Timeout == 0 {
		t.Fatalf("New(0) should default to non-zero timeout, got 0")
	}
	if c.httpc.Timeout < 1 {
		t.Fatalf("New(0) timeout should be reasonable, got %v", c.httpc.Timeout)
	}
}

func TestNewKeepsTimeout(t *testing.T) {
	c := New(5)
	if c.httpc.Timeout != 5 {
		t.Fatalf("New(5) should preserve timeout, got %v", c.httpc.Timeout)
	}
}
