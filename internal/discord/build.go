package discord

import (
	"io"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// fallbackBuildNumber is used when the live client build number cannot be
// fetched. Discord rejects logins whose build number is far behind the live
// client with an HTTP 403 and a captcha challenge, so this is kept reasonably
// current; the runtime fetch in resolveBuildNumber supersedes it when possible.
const fallbackBuildNumber = 573410

// buildAppURL is the SPA entry point whose HTML embeds the live build number.
// Discord removed BUILD_NUMBER from /api/login (now 404), so /app is the source.
const buildAppURL = "https://discord.com/app"

var buildNumberRegex = regexp.MustCompile(`"BUILD_NUMBER":\s*"(\d+)"`)

var (
	buildOnce   sync.Once
	buildNumber int
)

// clientBuildNumber returns the Discord client build number, fetching the live
// value once and memoizing it. On any failure it returns fallbackBuildNumber.
func clientBuildNumber() int {
	buildOnce.Do(func() {
		buildNumber = resolveBuildNumber()
	})
	return buildNumber
}

func resolveBuildNumber() int {
	n, err := fetchBuildNumber()
	if err != nil || n <= 0 {
		return fallbackBuildNumber
	}
	return n
}

func fetchBuildNumber() (int, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	return fetchBuildNumberWithClient(client, buildAppURL)
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func fetchBuildNumberWithClient(client httpDoer, appURL string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, appURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", clientBrowserUA)
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, err
	}
	m := buildNumberRegex.FindSubmatch(body)
	if len(m) < 2 {
		return 0, errNoBuildNumber
	}
	return strconv.Atoi(string(m[1]))
}
