package discord

import (
	"reflect"
	"testing"

	"github.com/diamondburned/arikawa/v3/utils/httputil"
)

type gifRequestRecorder struct {
	method string
	url    string
	result []GIFResult
}

func (r *gifRequestRecorder) RequestJSON(dst any, method, url string, _ ...httputil.RequestOption) error {
	r.method, r.url = method, url
	reflect.ValueOf(dst).Elem().Set(reflect.ValueOf(r.result))
	return nil
}

func TestSearchGIFsBuildsDiscordTenorRequest(t *testing.T) {
	r := &gifRequestRecorder{result: []GIFResult{{Title: "party", URL: "https://tenor.example/party.gif"}}}
	got, err := searchGIFs(r, "party cat")
	if err != nil {
		t.Fatal(err)
	}
	if r.method != "GET" {
		t.Fatalf("method = %q, want GET", r.method)
	}
	want := "https://discord.com/api/v9/gifs/search?q=party+cat&provider=tenor"
	if r.url != want {
		t.Fatalf("url = %q, want %q", r.url, want)
	}
	if len(got) != 1 || got[0].URL != r.result[0].URL {
		t.Fatalf("results = %+v", got)
	}
}

func TestSearchGIFsSkipsBlankQuery(t *testing.T) {
	r := &gifRequestRecorder{}
	got, err := searchGIFs(r, "  ")
	if err != nil || got != nil || r.url != "" {
		t.Fatalf("blank search = (%+v, %v), request %q", got, err, r.url)
	}
}
