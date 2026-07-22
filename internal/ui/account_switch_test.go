package ui

import (
	"testing"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/widget"
)

// TestResetAccountStateClearsComposerAndForum proves an account switch drops the
// previous account's reply/edit target, draft, attachments, and forum views, so
// no pending action can be routed through the newly active account.
func TestResetAccountStateClearsComposerAndForum(t *testing.T) {
	mv := &MainView{composer: widget.NewTextInput("Message")}
	mv.composer.SetValue("draft reply")
	mv.composerMode = composerReply
	mv.composerTarget = store.Message{ID: 5, ChannelID: 9}
	mv.replyMention = true
	mv.attachments = []queuedAttachment{{}}
	mv.forumActive = true
	mv.forumID = 42
	mv.forumPreviewID = 7
	mv.forumView = &ForumView{}

	mv.resetAccountState()

	if mv.composerMode != composerNormal || mv.composerTarget.ID != 0 || mv.composerTarget.ChannelID != 0 || mv.replyMention {
		t.Fatalf("reply state survived: mode=%v target=%+v mention=%v", mv.composerMode, mv.composerTarget, mv.replyMention)
	}
	if mv.composer.Value() != "" {
		t.Fatalf("composer draft survived: %q", mv.composer.Value())
	}
	if len(mv.attachments) != 0 {
		t.Fatalf("attachments survived: %d", len(mv.attachments))
	}
	if mv.forumActive || mv.forumID != 0 || mv.forumPreviewID != 0 || mv.forumView != nil {
		t.Fatalf("forum state survived: active=%v id=%d preview=%d view=%v",
			mv.forumActive, mv.forumID, mv.forumPreviewID, mv.forumView)
	}
}
