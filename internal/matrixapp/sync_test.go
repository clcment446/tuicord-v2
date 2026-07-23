package matrixapp

import "testing"

func TestStripReplyFallback(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no fallback", "hello world", "hello world"},
		{
			"single quote line",
			"> <@alice:example.org> original\n\nmy reply",
			"my reply",
		},
		{
			"multi quote lines",
			"> <@alice:example.org> line one\n> line two\n\nactual reply",
			"actual reply",
		},
		{"only quote", "> nothing after", "> nothing after"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stripReplyFallback(c.in); got != c.want {
				t.Fatalf("stripReplyFallback(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestLocalpart(t *testing.T) {
	if got := localpart("@alice:example.org"); got != "alice" {
		t.Fatalf("localpart = %q, want alice", got)
	}
	if got := localpart("bob"); got != "bob" {
		t.Fatalf("localpart = %q, want bob", got)
	}
}
