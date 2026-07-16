package app

import (
	"strconv"
	"testing"
)

func TestNewInteractionNonceIsDiscordSnowflake(t *testing.T) {
	nonce := newInteractionNonce()
	value, err := strconv.ParseUint(nonce, 10, 64)
	if err != nil || value == 0 {
		t.Fatalf("nonce %q is not a Discord snowflake: %v", nonce, err)
	}
	if len(nonce) < 17 || len(nonce) > 20 {
		t.Fatalf("nonce length = %d, want a snowflake-sized decimal string", len(nonce))
	}
}
