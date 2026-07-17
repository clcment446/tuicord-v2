package plugin

// Host is the set of side-effecting operations the tuicord Lua API can perform.
// It is a struct of functions rather than an interface so the wiring layer can
// supply closures that capture the app/UI without this package importing them,
// keeping internal/plugin free of any dependency on internal/app or internal/ui.
//
// Every function is called on the plugin goroutine and is itself responsible
// for marshalling any store/UI mutation onto the UI goroutine (via App.Post).
// IDs are passed as raw uint64 snowflakes; the Lua bindings parse the decimal
// strings plugins use before calling through.
type Host struct {
	// Send posts content to the active channel.
	Send func(content string)
	// SendTo posts content to a specific channel.
	SendTo func(channelID uint64, content string)
	// Reply posts content as a reply to a message, optionally pinging the author.
	Reply func(channelID, messageID uint64, content string, mention bool)
	// React adds an emoji reaction to a message. emoji is a unicode emoji or a
	// "name:id" custom-emoji reference.
	React func(channelID, messageID uint64, emoji string)
	// ActiveChannel returns the currently selected channel, or 0 if none.
	ActiveChannel func() uint64
	// ActiveGuild returns the currently selected guild, or 0 if none.
	ActiveGuild func() uint64
	// SelfID returns the logged-in user's ID, or 0 before READY.
	SelfID func() uint64
	// Notify surfaces a transient message to the user (a toast/notice).
	Notify func(title, body string)
}
