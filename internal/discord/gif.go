package discord

import (
	"net/url"
	"strings"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
)

// GIFResult is one result returned by Discord's Tenor proxy.
type GIFResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Src      string `json:"src"`
	ProxySrc string `json:"proxy_src"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type gifJSONRequester interface {
	RequestJSON(any, string, string, ...httputil.RequestOption) error
}

func searchGIFs(r gifJSONRequester, query string) ([]GIFResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	var results []GIFResult
	endpoint := strings.TrimSuffix(api.Endpoint, "/") + "/gifs/search?q=" + url.QueryEscape(query) + "&provider=tenor"
	if err := r.RequestJSON(&results, "GET", endpoint); err != nil {
		return nil, err
	}
	return results, nil
}

// SearchGIFs searches Discord's authenticated Tenor proxy. Sending a result's
// URL as ordinary message content matches the official client's behavior.
func SearchGIFs(client gifJSONRequester, query string) ([]GIFResult, error) {
	return searchGIFs(client, query)
}
