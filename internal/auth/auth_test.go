package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type memoryStore struct {
	token  string
	err    error
	saved  string
	setErr error
}

func (s *memoryStore) GetToken() (string, error) {
	return s.token, s.err
}

func (s *memoryStore) SetToken(token string) error {
	s.saved = token
	return s.setErr
}

func (s *memoryStore) DeleteToken() error {
	s.token = ""
	return nil
}

func TestResolveTokenPrefersStore(t *testing.T) {
	got, err := ResolveToken(context.Background(), Options{
		Store:  &memoryStore{token: "stored"},
		Getenv: func(string) string { return "env" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "stored" {
		t.Fatalf("got %q, want stored", got)
	}
}

func TestResolveTokenFallsBackToEnv(t *testing.T) {
	got, err := ResolveToken(context.Background(), Options{
		Store:  &memoryStore{err: errors.New("missing")},
		Getenv: func(string) string { return " env-token " },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "env-token" {
		t.Fatalf("got %q, want env-token", got)
	}
}

func TestResolveTokenPromptsAndSaves(t *testing.T) {
	store := &memoryStore{err: errors.New("missing")}
	got, err := ResolveToken(context.Background(), Options{
		Store:  store,
		Getenv: func(string) string { return "" },
		Prompt: func(context.Context) (string, error) {
			return " prompted-token ", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "prompted-token" {
		t.Fatalf("got %q, want prompted-token", got)
	}
	if store.saved != "prompted-token" {
		t.Fatalf("saved %q, want prompted-token", store.saved)
	}
}

func TestResolveTokenReturnsErrNoToken(t *testing.T) {
	_, err := ResolveToken(context.Background(), Options{
		Getenv: func(string) string { return "" },
	})
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("got %v, want ErrNoToken", err)
	}
}

func TestResolveTokenReturnsTokenWhenStoreIsUnavailable(t *testing.T) {
	storeErr := errors.New("The name is not activatable")
	store := &memoryStore{setErr: storeErr}
	var reported error

	got, err := ResolveToken(context.Background(), Options{
		Store: store,
		Prompt: func(context.Context) (string, error) {
			return "pasted-token", nil
		},
		OnStoreError: func(err error) { reported = err },
	})
	if err != nil {
		t.Fatalf("ResolveToken returned an error after successful login: %v", err)
	}
	if got != "pasted-token" {
		t.Fatalf("got %q, want pasted-token", got)
	}
	if reported == nil || !strings.Contains(reported.Error(), storeErr.Error()) {
		t.Fatalf("reported persistence error = %v, want %q", reported, storeErr)
	}
}
