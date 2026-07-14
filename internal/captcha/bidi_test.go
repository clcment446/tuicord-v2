package captcha

import (
	"encoding/json"
	"testing"
)

func TestBrowserExchangeResultDecodesJSONString(t *testing.T) {
	encoded := `{"status":200,"body":"{\"encrypted_token\":\"ciphertext\"}"}`
	var result struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != 200 || result.Body != `{"encrypted_token":"ciphertext"}` {
		t.Fatalf("result = %+v", result)
	}
}
