package term

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

// OpenURL launches an HTTP(S) URL in the user's default browser.
func OpenURL(target string) error {
	u, err := url.ParseRequestURI(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("invalid web URL %q", target)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}
