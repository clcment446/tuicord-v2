package term

import "testing"

func TestOpenURLRejectsNonWebSchemes(t *testing.T) {
	for _, target := range []string{"", "not a URL", "file:///tmp/a", "javascript:alert(1)"} {
		if err := OpenURL(target); err == nil {
			t.Errorf("OpenURL(%q) accepted a non-web URL", target)
		}
	}
}
