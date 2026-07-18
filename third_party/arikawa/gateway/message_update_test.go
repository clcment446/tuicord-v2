package gateway

import (
	"encoding/json"
	"testing"
)

func TestMessageUpdateEventTracksPartialFieldPresence(t *testing.T) {
	var event MessageUpdateEvent
	if err := json.Unmarshal([]byte(`{
		"id":"10","channel_id":"20",
		"content":"","flags":0,"attachments":[],"embeds":[],
		"sticker_items":[],"components":[],"pinned":false
	}`), &event); err != nil {
		t.Fatal(err)
	}
	if event.Fields == nil {
		t.Fatal("Fields is nil after JSON unmarshal")
	}
	if !event.Fields.Content || !event.Fields.Flags || !event.Fields.Attachments ||
		!event.Fields.Embeds || !event.Fields.Stickers || !event.Fields.Components || !event.Fields.Pinned {
		t.Fatalf("presence = %+v, want every patch field present", *event.Fields)
	}
	if event.Content != "" || event.Flags != 0 || event.Pinned || event.Attachments == nil ||
		event.Embeds == nil || event.Stickers == nil || event.Components == nil {
		t.Fatalf("explicit empty values were not preserved: %+v", event.Message)
	}
}

func TestMessageUpdateEventDistinguishesOmittedFields(t *testing.T) {
	var event MessageUpdateEvent
	if err := json.Unmarshal([]byte(`{"id":"10","channel_id":"20","embeds":[]}`), &event); err != nil {
		t.Fatal(err)
	}
	if event.Fields == nil || !event.Fields.Embeds {
		t.Fatalf("presence = %+v, want embeds present", event.Fields)
	}
	if event.Fields.Content || event.Fields.Flags || event.Fields.Attachments ||
		event.Fields.Stickers || event.Fields.Components || event.Fields.Pinned {
		t.Fatalf("presence = %+v, omitted fields reported present", *event.Fields)
	}
}
