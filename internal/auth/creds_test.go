package auth

import "testing"

func TestCredentialsRoundTrip(t *testing.T) {
	in := Credentials{
		Protocol:    ProtocolMatrix,
		Homeserver:  "https://matrix-client.matrix.org",
		UserID:      "@alice:matrix.org",
		DeviceID:    "ABCDEF",
		AccessToken: "syt_token",
		PickleKey:   []byte("0123456789abcdef0123456789abcdef"),
	}
	enc, err := in.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, ok := Decode(enc)
	if !ok {
		t.Fatal("Decode returned ok=false for a valid blob")
	}
	if got.UserID != in.UserID || got.AccessToken != in.AccessToken || got.Homeserver != in.Homeserver {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if string(got.PickleKey) != string(in.PickleKey) {
		t.Fatalf("pickle key not preserved")
	}
}

func TestDecodeLegacyDiscordToken(t *testing.T) {
	// A bare Discord token is not JSON and must not decode as Matrix credentials.
	if _, ok := Decode("mfa.some-discord-token-value"); ok {
		t.Fatal("bare token decoded as Matrix credentials")
	}
	if _, ok := Decode(""); ok {
		t.Fatal("empty string decoded as credentials")
	}
	// JSON lacking a protocol field is also rejected.
	if _, ok := Decode(`{"access_token":"x"}`); ok {
		t.Fatal("protocol-less JSON decoded as credentials")
	}
}
