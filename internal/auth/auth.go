package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"awesomeProject/internal/keyring"
)

const TokenEnv = "TOKEN"

var ErrNoToken = errors.New("auth token not found")

type TokenStore interface {
	GetToken() (string, error)
	SetToken(token string) error
	DeleteToken() error
}

type PromptFunc func(context.Context) (string, error)

type Options struct {
	Store        TokenStore
	Getenv       func(string) string
	Prompt       PromptFunc
	OnStoreError func(error)
}

func ResolveToken(ctx context.Context, opts Options) (string, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	if opts.Store != nil {
		token, err := opts.Store.GetToken()
		if err != nil && !errors.Is(err, keyring.ErrNotFound) && opts.OnStoreError != nil {
			// A read failure other than "nothing stored" must be surfaced: it means
			// a saved token may exist but is unreachable, and silently falling
			// through to a fresh login would mask the broken keyring.
			opts.OnStoreError(fmt.Errorf("read auth token: %w", err))
		}
		if token = strings.TrimSpace(token); err == nil && token != "" {
			return token, nil
		}
	}

	if token := strings.TrimSpace(getenv(TokenEnv)); token != "" {
		return token, nil
	}

	if opts.Prompt == nil {
		return "", ErrNoToken
	}

	token, err := opts.Prompt(ctx)
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrNoToken
	}

	if opts.Store != nil {
		if err := opts.Store.SetToken(token); err != nil {
			if opts.OnStoreError != nil {
				opts.OnStoreError(fmt.Errorf("save auth token: %w", err))
			}
		}
	}

	return token, nil
}

func ForgetToken(store TokenStore) error {
	if store == nil {
		return nil
	}
	return store.DeleteToken()
}
