//go:build windows

package term

import "time"

func queryTerminal(int, time.Duration) string {
	return ""
}
