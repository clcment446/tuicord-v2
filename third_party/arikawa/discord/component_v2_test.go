package discord

import (
	"encoding/json"
	"testing"
)

func TestMessageHistoryDecodesTopLevelTextDisplay(t *testing.T) {
	const payload = `[{"id":"1","channel_id":"2","flags":32768,"components":[{"type":10,"id":1,"content":"# Welcome"}]}]`

	var messages []Message
	if err := json.Unmarshal([]byte(payload), &messages); err != nil {
		t.Fatalf("decode message history with top-level text display: %v", err)
	}
	if len(messages) != 1 || len(messages[0].Components) != 1 {
		t.Fatalf("decoded messages = %+v, want one message with one component", messages)
	}
	text, ok := messages[0].Components[0].(*TextDisplayComponent)
	if !ok || text.Content != "# Welcome" {
		t.Fatalf("top-level component = %#v, want TextDisplayComponent", messages[0].Components[0])
	}
}
