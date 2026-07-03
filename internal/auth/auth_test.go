package auth

import (
	"context"
	"errors"
	"testing"
)

type memoryStore struct {
	token string
	err   error
	saved string
}

func (s *memoryStore) GetToken() (string, error) {
	return s.token, s.err
}

func (s *memoryStore) SetToken(token string) error {
	s.saved = token
	return nil
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
