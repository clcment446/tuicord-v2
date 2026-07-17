package markup

import (
	"strings"
	"testing"

	"awesomeProject/internal/store"
)

func TestExportMessagePreservesContentAndExportsRichDataAsMarkdown(t *testing.T) {
	message := store.Message{
		Content: "**bold** [site](https://example.test) <@42> <:wave:7> ||secret||",
		Attachments: []store.Attachment{{
			Filename: "report.pdf", URL: "https://cdn.test/report.pdf", ContentType: "application/pdf", Size: 2048,
		}},
		Stickers: []store.Sticker{{ID: 9, Name: "party", Format: store.StickerGIF}},
		Embeds: []store.Embed{{
			AuthorName: "Example", Title: "Release", URL: "https://example.test/release",
			Description: "**shipped**", Fields: []store.EmbedField{{Name: "Status", Value: "green"}},
			FooterText: "today", ImageURL: "https://cdn.test/image.png", Provider: "Example Inc.",
		}},
		ComponentTree: []store.ComponentNode{{
			Kind: store.ComponentContainer,
			Children: []store.ComponentNode{
				{Kind: store.ComponentTextDisplay, Content: "## Details\nReady"},
				{Kind: store.ComponentButton, Label: "Approve", CustomID: "approve"},
				{Kind: store.ComponentMediaGallery, Media: []store.ComponentMedia{{Name: "demo.png", URL: "https://cdn.test/demo.png", Description: "Demo"}}},
			},
		}},
	}

	got := ExportMessage(message)
	for _, want := range []string{
		message.Content,
		"## Attachments\n- [report.pdf](https://cdn.test/report.pdf) (application/pdf, 2 KiB)",
		"## Stickers\n- party (GIF)",
		"## Embeds\n### [Release](https://example.test/release)",
		"**Author:** Example", "**shipped**", "- **Status:** green", "_today_", "**Provider:** Example Inc.", "![image](https://cdn.test/image.png)",
		"## Components\n## Details\nReady", "- Button: Approve", "- Media: [Demo](https://cdn.test/demo.png)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ExportMessage() missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\"ComponentTree\"") || strings.Contains(got, "{\"") {
		t.Fatalf("ExportMessage() emitted JSON: %s", got)
	}
}

func TestExportMessageUsesLegacyComponentsAndHandlesMissingOptionalValues(t *testing.T) {
	got := ExportMessage(store.Message{Components: []store.Component{
		{Kind: store.ComponentLinkButton, Label: "Docs", URL: "https://docs.test"},
		{Kind: store.ComponentSelect, Label: "Pick"},
	}})
	want := "## Components\n- Link button: [Docs](https://docs.test)\n- Select: Pick"
	if got != want {
		t.Fatalf("ExportMessage() = %q, want %q", got, want)
	}
}

func TestExportMessageEmptyMessageIsEmpty(t *testing.T) {
	if got := ExportMessage(store.Message{}); got != "" {
		t.Fatalf("ExportMessage() = %q, want empty", got)
	}
}

func TestExportMessagesSeparatesFormattedMessages(t *testing.T) {
	got := ExportMessages([]store.Message{{Content: "first"}, {Content: "second"}})
	if want := "first\n\n---\n\nsecond"; got != want {
		t.Fatalf("ExportMessages() = %q, want %q", got, want)
	}
}
