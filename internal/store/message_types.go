// Package store — rich message content types.
//
// This file declares the supplementary types that extend [Message] with
// media, embed, sticker, reaction, and interactive-component data delivered
// by the Discord gateway and REST API.
package store

// Attachment is a file or media item attached to a Discord message.
type Attachment struct {
	// URL is the canonical CDN URL.
	URL string
	// ProxyURL is the Discord media-proxy URL (preferred for fetches; Discord
	// applies CDN resizing via ?width=&height=&format= query parameters).
	ProxyURL string
	// Filename is the original uploaded filename.
	Filename string
	// ContentType is the MIME type, e.g. "image/png" or "video/mp4".
	ContentType string
	// W and H are the pixel dimensions for images and videos (0 if unknown).
	W, H int
	// Size is the file size in bytes.
	Size int64
}

// EmbedKind identifies the type of a V1 Discord embed.
type EmbedKind int

const (
	// EmbedRich is a generic embed with arbitrary structured fields.
	EmbedRich EmbedKind = iota
	// EmbedImage is an embed whose primary content is a single image.
	EmbedImage
	// EmbedVideo is an embed whose primary content is a video.
	EmbedVideo
	// EmbedGIFV is an animated GIF embed (tenor, giphy, klipy, …).
	EmbedGIFV
	// EmbedLink is a minimal URL-unfurl embed.
	EmbedLink
)

// EmbedField is a name/value pair inside a rich embed.
type EmbedField struct {
	// Name is the field label.
	Name string
	// Value is the field body (may contain Discord markup).
	Value string
	// Inline, when true, hints that the field should render in a multi-column
	// grid alongside other inline fields.
	Inline bool
}

// Embed is a normalized V1 Discord embed attached to a message. Discord
// delivers embeds asynchronously after MESSAGE_CREATE once URLs are unfurled,
// so the store must support patching an existing message via UpdateMessage.
type Embed struct {
	// Kind categorizes the embed for rendering decisions.
	Kind EmbedKind
	// Color is the left-gutter accent color (0xRRGGBB; 0 = default).
	Color uint32
	// AuthorName is the optional author display line.
	AuthorName string
	// Title is the embed headline.
	Title string
	// URL is the link target for a clickable Title.
	URL string
	// Description is the body text (may contain Discord markup).
	Description string
	// Fields is the ordered list of name/value pairs.
	Fields []EmbedField
	// FooterText is the small dim footer line.
	FooterText string
	// ImageURL is the large bottom-image URL.
	ImageURL string
	// ThumbURL is the small thumbnail URL (top-right corner).
	ThumbURL string
	// VideoURL is the video source URL.
	VideoURL string
	// Provider is the name of the external provider, e.g. "YouTube".
	Provider string
}

// StickerFormat identifies the file format of a Discord sticker.
type StickerFormat int

const (
	// StickerPNG is a static PNG sticker.
	StickerPNG StickerFormat = iota
	// StickerAPNG is an animated PNG sticker (rendered as first frame v1).
	StickerAPNG
	// StickerGIF is an animated GIF sticker.
	StickerGIF
	// StickerLottie is a Lottie JSON animation; it cannot be decoded in-terminal
	// and falls back to a "[sticker: name]" chip.
	StickerLottie
)

// Sticker is a Discord sticker attached to a message.
type Sticker struct {
	// ID is the sticker's snowflake identifier.
	ID uint64
	// Name is the sticker's display name.
	Name string
	// Format is the sticker's file encoding.
	Format StickerFormat
}

// Reaction is a single emoji reaction entry on a message.
type Reaction struct {
	// EmojiName is the Unicode emoji or custom emoji name.
	EmojiName string
	// EmojiID is the custom emoji snowflake (0 for Unicode emoji).
	EmojiID uint64
	// Animated reports whether the custom emoji is animated.
	Animated bool
	// Count is the total number of users who reacted with this emoji.
	Count int
	// Me reports whether the current user has applied this reaction.
	Me bool
}

// ComponentKind identifies the type of an interactive message component.
type ComponentKind int

const (
	// ComponentButton is a pressable inline button.
	ComponentButton ComponentKind = iota
	// ComponentLinkButton is a button that opens an external URL.
	ComponentLinkButton
	// ComponentSelect is a drop-down select menu (rendered disabled in v1).
	ComponentSelect
	// ComponentActionRow groups interactive children horizontally.
	ComponentActionRow
	// ComponentContainer groups rich V2 children under an optional accent.
	ComponentContainer
	// ComponentSection pairs text children with a thumbnail or button accessory.
	ComponentSection
	// ComponentTextDisplay is rich markdown-like text inside a V2 message.
	ComponentTextDisplay
	// ComponentThumbnail is a compact media accessory.
	ComponentThumbnail
	// ComponentMediaGallery is a collection of one or more media items.
	ComponentMediaGallery
	// ComponentFile is a downloadable file component.
	ComponentFile
	// ComponentSeparator is vertical spacing and an optional divider.
	ComponentSeparator
	// ComponentUnknown is a component type the client does not yet understand.
	ComponentUnknown
	// ComponentTextInput is a text input field inside a modal-like layout.
	ComponentTextInput
	// ComponentLabel wraps an input with label and description text.
	ComponentLabel
	// ComponentFileUpload is a file picker/upload input.
	ComponentFileUpload
	// ComponentRadioGroup is a single-choice option list.
	ComponentRadioGroup
	// ComponentCheckboxGroup is a multi-choice option list.
	ComponentCheckboxGroup
	// ComponentCheckbox is a single boolean input.
	ComponentCheckbox
)

// Component is a single interactive V2 message component (button, link, or
// select menu). Components arrive nested inside action rows; the store
// flattens them into a single slice per message.
type Component struct {
	// Kind determines how the component is rendered and activated.
	Kind ComponentKind
	// Label is the display text shown on the component.
	Label string
	// CustomID is the developer-assigned identifier sent back on interaction.
	CustomID string
	// Style is the numeric Discord button style (primary=1, danger=4, …).
	Style int
	// URL is the navigation target for ComponentLinkButton.
	URL string
	// Disabled reports whether the component is non-interactive.
	Disabled bool
}

// ComponentState tracks local UI feedback for an interactive component.
type ComponentState int

const (
	// ComponentStateIdle is the default, ready state.
	ComponentStateIdle ComponentState = iota
	// ComponentStatePending means the user activated the control and is waiting
	// for acknowledgement.
	ComponentStatePending
	// ComponentStateSuccess means the last activation completed successfully.
	ComponentStateSuccess
	// ComponentStateError means the last activation failed.
	ComponentStateError
)

// ComponentOption is one selectable option inside a string select menu.
type ComponentOption struct {
	Label       string
	Value       string
	Description string
	Default     bool
}

// ComponentMedia is a media or file payload referenced by a V2 component.
type ComponentMedia struct {
	URL         string
	ProxyURL    string
	Description string
	ContentType string
	W, H        int
	Size        int64
	Spoiler     bool
	Name        string
}

// ComponentNode is a normalized hierarchical Discord component. It supports
// Components V2 layouts while still representing legacy action rows.
type ComponentNode struct {
	Kind ComponentKind
	// RawType keeps the Discord component type number for unknown/fallback
	// rendering and debugging.
	RawType int
	// ID is the optional numeric component ID used by Components V2.
	ID int

	CustomID    string
	Label       string
	Description string
	Content     string
	Placeholder string
	Style       int
	URL         string
	Disabled    bool
	State       ComponentState
	InputField  bool
	Required    bool
	Value       string
	Values      []string
	MinValues   int
	MaxValues   int

	AccentColor uint32
	Spoiler     bool
	Divider     bool
	Spacing     int

	Options []ComponentOption
	Media   []ComponentMedia

	Children  []ComponentNode
	Accessory *ComponentNode
}
