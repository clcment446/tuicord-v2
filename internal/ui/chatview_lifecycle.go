package ui

import (
	"awesomeProject/internal/config"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/tui"
	"context"
	"image"
)

// OnReachTop registers a callback invoked when the user scrolls toward older
// messages. The callback runs on the UI goroutine.
func (w *ChatView) OnReachTop(fn func()) {
	if w != nil {
		w.onReachTop = fn
	}
}

// SetStyles refreshes the palette used for subsequent renders. The shared
// generation makes cached message bodies miss and repaint in the new theme.
func (w *ChatView) SetStyles(styles Styles) {
	if w != nil {
		w.styles = styles
		w.invalidateBodies()
	}
}

// SetSource rebinds the transcript to a different account's store and
// active-channel accessor (used on multi-account switch) and resets the
// interactive view state — scroll, focus, and selection — so nothing from the
// previous account's messages leaks into the newly shown one. URL-keyed media
// state is left intact since attachment URLs are global.
func (w *ChatView) SetSource(st *store.Store, active func() store.ChannelID) {
	if w == nil {
		return
	}
	w.store = st
	w.active = active
	w.invalidateBodies()
	w.bottomScroll.SetOffset(0)
	w.focusedMessageSet = false
	w.focusedExplicit = false
	w.focusKey = ""
	w.focusStopKey = ""
	w.focusStopIndex = -1
	w.selectionActive = false
	w.selectionStart = 0
	w.contextMessageSet = false
	w.headerMessageKey = ""
	w.vimPendingG = false
}

// SetMedia enables asynchronous inline media loading for attachments, stickers,
// emoji CDN links, and image embeds. post must schedule callbacks on the UI
// goroutine; passing nil leaves text-chip fallbacks in place.
func (w *ChatView) SetMedia(fetcher *media.Fetcher, cfg media.Config, post func(func())) {
	if w == nil {
		return
	}
	if w.mediaCancel != nil {
		w.mediaCancel()
		w.mediaWG.Wait()
	}
	cfg = cfg.Bounded()
	w.mediaFetcher = fetcher
	w.mediaCfg = cfg
	w.post = post
	w.mediaEpoch++
	w.invalidateBodies()
	if w.media == nil {
		w.media = map[string]*chatMediaState{}
	}
	w.mediaCtx, w.mediaCancel = context.WithCancel(context.Background())
	w.mediaJobs = make(chan chatMediaJob, cfg.QueuedFetches)
	if fetcher == nil || post == nil || !cfg.Enabled {
		return
	}
	for range cfg.ConcurrentFetches {
		w.mediaWG.Add(1)
		go w.mediaWorker(w.mediaCtx, w.mediaJobs)
	}
}

// CloseMedia cancels queued and in-flight chat media requests. Workers use the
// same context for queue waits and HTTP, so none can remain blocked behind a
// semaphore after the view is closed.
func (w *ChatView) CloseMedia() {
	if w != nil && w.mediaCancel != nil {
		w.mediaCancel()
		w.mediaCancel = nil
		w.mediaWG.Wait()
	}
}

// SetRoleGradients opts author names into cached Discord role gradients. The
// animation option only repaints while a gradient author is visible.
func (w *ChatView) SetRoleGradients(enabled, animate bool) {
	if w == nil {
		return
	}
	w.roleGradients = enabled
	w.roleGradientAnimations = enabled && animate
}

// Measure fills available space.
func (w *ChatView) Measure(avail tui.Size) tui.Size { return avail }

// Layout returns the layout node.
func (w *ChatView) Layout() *layout.Node { return &w.node }

// CanFocus lets the chat view take focus for scrolling.
func (w *ChatView) CanFocus() bool { return true }

// PreferredFocus starts an opted-in Vim session on message navigation rather
// than in the composer.
func (w *ChatView) PreferredFocus() bool { return w != nil && w.vimNavigation }

func (w *ChatView) VimFocusEnabled() bool { return w != nil && w.vimNavigation }

// SetFocusOwner records whether the chat panel itself owns keyboard focus.
// Component shortcut labels are deliberately hidden while another panel owns
// focus, preventing number keys typed elsewhere from looking actionable here.
func (w *ChatView) SetFocusOwner(focused bool) {
	if w == nil || w.keyboardFocused == focused {
		return
	}
	w.keyboardFocused = focused
	if w.focusedMessageSet {
		w.invalidateMsgs(w.focusedMessage)
	}
}

// SetVimNavigation enables modal hjkl/message actions for this chat view.
// It is disabled by default so ordinary letter input remains inert outside
// explicit Vim configurations.
func (w *ChatView) SetVimNavigation(enabled bool) {
	if w != nil {
		w.vimNavigation = enabled
		if w.vimKeys == (config.VimKeys{}) {
			w.vimKeys = config.Default().Keys.Vim
		}
	}
}

// SetVimKeys replaces the modal action map used by this view.
func (w *ChatView) SetVimKeys(keys config.VimKeys) {
	if w != nil {
		if keys == (config.VimKeys{}) {
			keys = config.Default().Keys.Vim
		}
		w.vimKeys = keys
	}
}

// SetMouseBreakpointTracking opts pointer motion into changing the keyboard
// stopping point. Click activation remains available regardless of this flag.
func (w *ChatView) SetMouseBreakpointTracking(enabled bool) {
	if w != nil {
		w.mouseBreakpointTracking = enabled
	}
}

// SetHighlightFocusBlock expands the focused anchor across its full logical
// block, through the line before the next stopping point.
func (w *ChatView) SetHighlightFocusBlock(enabled bool) {
	if w != nil {
		w.highlightFocusBlock = enabled
	}
}

// OnMessageAction receives D/R/E/A actions for the focused message row.
func (w *ChatView) OnMessageAction(fn func(rune, store.Message)) { w.onMessageAction = fn }

// OnMessageCopy receives the messages selected through Vim visual mode.
func (w *ChatView) OnMessageCopy(fn func([]store.Message)) { w.onMessageCopy = fn }

// OnMessageFocus receives the message under the keyboard or mouse focus bar.
func (w *ChatView) OnMessageFocus(fn func(store.Message)) { w.onMessageFocus = fn }

// OnPlayVideo registers the callback that starts inline video playback. The
// region is in absolute terminal cells.
func (w *ChatView) OnPlayVideo(fn func(url string, region media.Rect)) { w.onPlayVideo = fn }

// OnStopVideo registers the callback that tears down the current playback.
func (w *ChatView) OnStopVideo(fn func()) { w.onStopVideo = fn }

// OnOpenMedia registers the callback that opens an image/GIF in the viewer.
func (w *ChatView) OnOpenMedia(fn func(url string, img image.Image, frames []media.Frame)) {
	w.onOpenMedia = fn
}

// SetInvalidate registers the repaint hook used to surface loaded media promptly.
func (w *ChatView) SetInvalidate(fn func()) { w.requestRedraw = fn }

func (w *ChatView) invalidate() {
	if w.requestRedraw != nil {
		w.requestRedraw()
	}
}

// Animating reports whether a visible GIF needs the fast animation tick. The
// runtime reads it to raise the tick cadence only while something is moving.
func (w *ChatView) Animating() bool { return w != nil && w.animatedVisible }
