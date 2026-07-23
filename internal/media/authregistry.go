package media

import (
	"net/http"
	"sync"
)

// FetchAuthorizer decorates outgoing media requests and post-processes the
// downloaded bytes for a specific protocol (e.g. Matrix authenticated media and
// encrypted attachments). It keeps the media package protocol-neutral: the
// Matrix backend registers an implementation, and Discord URLs — which no
// authorizer claims — are fetched exactly as before.
type FetchAuthorizer interface {
	// Authorize reports whether it handles url. When it does, it may set request
	// headers (returned in header) and/or return a transform applied to the
	// downloaded body (used to decrypt encrypted attachments). A nil transform
	// means the bytes are used as-is.
	Authorize(url string) (header http.Header, transform func([]byte) ([]byte, error), handled bool)
}

var (
	authMu      sync.RWMutex
	authorizers []FetchAuthorizer
)

// RegisterAuthorizer adds a media fetch authorizer. Registration is additive and
// idempotent per instance; the first authorizer that claims a URL wins.
func RegisterAuthorizer(a FetchAuthorizer) {
	if a == nil {
		return
	}
	authMu.Lock()
	defer authMu.Unlock()
	for _, existing := range authorizers {
		if existing == a {
			return
		}
	}
	authorizers = append(authorizers, a)
}

// resolveAuth returns the header/transform for url from the first authorizer
// that claims it, or (nil, nil) when none does.
func resolveAuth(url string) (http.Header, func([]byte) ([]byte, error)) {
	authMu.RLock()
	defer authMu.RUnlock()
	for _, a := range authorizers {
		if header, transform, ok := a.Authorize(url); ok {
			return header, transform
		}
	}
	return nil, nil
}
