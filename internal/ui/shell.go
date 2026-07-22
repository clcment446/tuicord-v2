package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/plugin"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/term"
	"awesomeProject/internal/tui/text"
	tuitext "awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// PluginHost is the slice of the Lua plugin manager the Shell uses to dispatch
// user input to plugins. It is optional; a nil host disables all plugin
// dispatch. Implemented by *plugin.Manager.
type PluginHost interface {
	// RunCommand runs a plugin ;-command, reporting whether dispatch was accepted.
	RunCommand(name string, args []string) bool
	// RunKey runs a plugin key binding, reporting whether dispatch was accepted.
	RunKey(spec string) bool
	// KeySpecs lists the key specs plugins have bound.
	KeySpecs() []string
	// CommandNames lists registered plugin command names for help.
	CommandNames() []string
	// ApplyTheme applies a registered theme by name, reporting whether it exists.
	ApplyTheme(name string) bool
	// ThemeNames lists registered theme names.
	ThemeNames() []string
}

// editorInteractionPhase is the single authority for Vim composer ownership.
// MainView reads it through a callback only for status rendering; it does not
// maintain a second mode flag.
type editorInteractionPhase uint8

const (
	editorNormal editorInteractionPhase = iota
	editorFocusPending
	editorInput
	editorOverlaySuspended
)

type editorInteraction struct {
	phase            editorInteractionPhase
	channel          store.ChannelID
	allowReplacement bool
}

// Shell is the root widget. It shows the main view and can swap in a
// full-screen overlay (quick switcher or help). Overlays are implemented as a
// tree swap rather than a z-ordered layer, which the toolkit supports directly:
// Children returns whichever subtree is active, so focus, hit-testing, and
// drawing all follow.
type Shell struct {
	mv             *MainView
	app            *app.App
	cfg            config.Config
	styles         Styles
	plugins        PluginHost
	overlay        tui.Widget // nil = show the main view
	popup          tui.Widget // small interactive layer drawn over current()
	toasts         []*Toast
	notifier       desktopNotifier
	dispatch       func(func())
	post           func(func())
	tryPost        func(func()) bool
	unfocused      bool
	now            func() time.Time
	forumPreview   *forumPreview
	completionSync bool
	commandLoading bool

	editor             editorInteraction
	composerOverlay    tui.Widget
	focusGeneration    uint64
	focusRequest       tui.FocusRequest
	focusRequestReady  bool
	focusRequestActive bool
	focusTraversal     int
	prefetch           *idleMediaPrefetcher
	profileFetcher     *media.Fetcher
	prefetchIdle       bool
	lastActivity       time.Time
	cancel             context.CancelFunc
	lifecycleCtx       context.Context
	lifecycleCancel    context.CancelFunc
	viewerCancel       context.CancelFunc
	clipboardMu        sync.Mutex
	clipboardCancel    context.CancelFunc
	clipboardBusy      bool
	lifecycleWG        sync.WaitGroup
	closeOnce          sync.Once
	mediaCfg           media.Config
	node               layout.Node
	video              *media.VideoPlayer
	videoRegion        media.Rect
}

// SetPluginHost registers the Lua plugin manager for command and key dispatch.
func (s *Shell) SetPluginHost(host PluginHost) { s.plugins = host }

// SetStyles refreshes the Shell's palette for new notices, menus, and overlays,
// and updates the straightforward retained MainView surfaces.
func (s *Shell) SetStyles(styles Styles) {
	if s == nil {
		return
	}
	s.styles = styles
	if s.mv != nil {
		s.mv.SetStyles(styles)
	}
	if menu, ok := s.popup.(*widget.Menu); ok {
		s.styleMenu(menu)
	}
}

// OpenPluginOverlay shows a read-only panel of plugin-supplied text lines. It
// swaps in a full-screen overlay dismissed with Esc, like the help panel. Call
// on the UI goroutine.
func (s *Shell) OpenPluginOverlay(title string, lines []string) {
	textStyle := s.styles.Cell("messages.content")
	rows := make([]tui.Widget, 0, len(lines))
	for _, line := range lines {
		t := widget.NewText(line)
		t.SetStyle(textStyle)
		rows = append(rows, t)
	}
	if len(rows) == 0 {
		empty := widget.NewText("")
		empty.SetStyle(textStyle)
		rows = append(rows, empty)
	}
	border := titled(title, widget.Column(rows...))
	border.SetStyle(s.styles.Cell("panels.border"))
	border.SetFocusStyle(s.styles.Cell("panels.focus"))
	s.setIndependentOverlay(border)
}

// OpenPluginViewport shows a compact interactive plugin panel over the active
// view. Reopening it replaces only the previous floating panel, never chat.
func (s *Shell) OpenPluginViewport(title string, lines []string, actions []plugin.ViewportAction, onAction func(string)) {
	if s == nil {
		return
	}
	// This status surface is intentionally non-modal: opening it must preserve
	// Vim INSERT mode and leave the composer focused.
	viewport := newPluginViewport(title, lines, actions, onAction, s.styles)
	if previous, ok := s.popup.(*pluginViewport); ok && previous.last.W > 0 && previous.last.H > 0 {
		// Plugins commonly refresh a viewport by opening it again. Retain its
		// rendered geometry so a data update never recenters a dragged/resized
		// panel under the user's pointer.
		viewport.modal.SetSize(previous.last.W, previous.last.H)
		viewport.modal.SetPosition(previous.last.X, previous.last.Y)
	}
	s.popup = viewport
}

type desktopNotifier interface {
	Notify(title, body string) error
}

type systemDesktopNotifier struct{}

const desktopNotificationTimeout = 2 * time.Second

func (systemDesktopNotifier) Notify(title, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), desktopNotificationTimeout)
	defer cancel()
	switch runtime.GOOS {
	case "linux":
		return exec.CommandContext(ctx, "notify-send", "--app-name=Tuicord", title, body).Run()
	case "darwin":
		const script = `on run argv
	display notification (item 1 of argv) with title (item 2 of argv)
end run`
		return exec.CommandContext(ctx, "osascript", "-e", script, "--", body, title).Run()
	default:
		return fmt.Errorf("desktop notifications are unavailable on %s", runtime.GOOS)
	}
}

// NewShell wraps a MainView with overlay handling.
func NewShell(a *app.App, mv *MainView, cfg config.Config, styles Styles, cancel context.CancelFunc) *Shell {
	if cfg.Keys.Vim == (config.VimKeys{}) {
		cfg.Keys.Vim = config.Default().Keys.Vim
	}
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	mediaCfg := chatMediaConfig(cfg)
	s := &Shell{mv: mv, app: a, cfg: cfg, styles: styles, cancel: cancel, lifecycleCtx: lifecycleCtx, lifecycleCancel: lifecycleCancel, mediaCfg: mediaCfg, lastActivity: time.Now(), now: time.Now, notifier: systemDesktopNotifier{}, dispatch: func(fn func()) { go fn() }, post: a.Post, tryPost: a.TryPost, node: layout.Node{Grow: 1}}
	if mediaCfg.Enabled && mediaCfg.Prefetch {
		s.prefetch = newIdleMediaPrefetcher(newChatMediaFetcher(mediaCfg))
	}
	s.video = media.NewVideoPlayer(mediaCfg)
	mv.chat.OnPlayVideo(s.playVideo)
	mv.chat.OnStopVideo(s.teardownVideo)
	mv.chat.OnOpenMedia(s.openMediaViewer)
	mv.onNewForumPost = s.openForumPostPrompt
	mv.onForumFilter = s.openForumFilterMenu
	mv.onForumHover = s.setForumHover
	s.bindEditorState()
	mv.SetComposerChange(s.composerChanged)
	mv.composer.OnBackspaceEmpty(s.leaveInputMode)
	mv.SetLocalCommandHandler(s.runLocalCommand)
	mv.SetChannelChangeHandler(s.channelChanged)
	mv.chat.OnMessageAction(s.handleMessageAction)
	// ForumView is created lazily; MainView invokes this hook when it exists.
	return s
}

// handleMessageAction reconnects ChatView's Vim message shortcuts to the
// shell-owned actions. This callback was lost in the merge, leaving r/e/u/d/a
// visually handled but with no effect.
func (s *Shell) handleMessageAction(action rune, msg store.Message) {
	if s == nil || s.mv == nil {
		return
	}
	switch action {
	case 'r':
		s.mv.BeginReply(msg, true)
		s.focusComposer()
	case 'e':
		if s.app != nil && msg.AuthorID != 0 && msg.AuthorID == s.app.SelfID() {
			s.mv.BeginEdit(msg)
			s.focusComposer()
		}
	case 'u':
		if msg.AuthorID != 0 {
			s.openMessageAuthorProfile(msg)
		}
	case 'd':
		if s.app != nil && msg.AuthorID != 0 && msg.AuthorID == s.app.SelfID() {
			s.app.DeleteMessage(msg.ChannelID, msg.ID)
		}
	}
}

// SetActiveAccount rebinds the shell to a different account's orchestrator on a
// multi-account switch. Every s.app read is dynamic, and post/tryPost forward to
// the shared tui runtime (identical across accounts), so swapping the pointer is
// sufficient. Call on the UI goroutine.
func (s *Shell) SetActiveAccount(a *app.App) {
	if s == nil || a == nil {
		return
	}
	s.app = a
}

func (s *Shell) bindEditorState() {
	if s == nil || s.mv == nil {
		return
	}
	s.mv.composerInputActive = s.editorStatusActive
}

func (s *Shell) editorStatusActive() bool {
	return s != nil && s.cfg.Accessibility.VimNavigation && s.editor.phase != editorNormal
}

func (s *Shell) activeChannel() store.ChannelID {
	if s == nil || s.app == nil {
		return 0
	}
	return s.app.ActiveChannel()
}

func (s *Shell) composerWritable() bool {
	return s != nil && s.mv != nil && s.mv.composer != nil && !s.mv.composer.ReadOnly()
}

func (s *Shell) focusComposer() {
	if s == nil || !s.composerWritable() {
		if s != nil && s.cfg.Accessibility.VimNavigation {
			s.exitEditor(nil)
		}
		return
	}
	s.bindEditorState()
	if s.cfg.Accessibility.VimNavigation {
		s.beginComposerInput(false)
		return
	}
	s.queueFocus(s.mv.composer)
}

func (s *Shell) beginComposerInput(allowReplacement bool) bool {
	if s == nil || !s.cfg.Accessibility.VimNavigation || !s.composerWritable() || s.overlay != nil {
		return false
	}
	// A plugin viewport is a non-modal popup and may be refreshed while the
	// focus ring is being rebuilt. In that case the rebuild can briefly choose
	// chat before the pending composer request is applied; tolerate that single
	// replacement so pressing Vim's insert key still reaches the composer.
	if _, ok := s.popup.(*pluginViewport); ok {
		allowReplacement = true
	}
	s.bindEditorState()
	s.cancelFocusRequest()
	s.editor = editorInteraction{
		phase:            editorFocusPending,
		channel:          s.activeChannel(),
		allowReplacement: allowReplacement,
	}
	s.mv.composer.SetInputFocusEnabled(true)
	s.queueFocus(s.mv.composer)
	s.mv.updateComposerStatus()
	return true
}

// leaveInputMode is an explicit Vim exit used by ;q and empty Backspace. It
// preserves the draft, attachments, and reply/edit operation while returning
// focus to message navigation.
func (s *Shell) leaveInputMode() {
	if s == nil || s.editor.phase == editorNormal {
		return
	}
	s.exitEditor(s.mv.chat)
}

func (s *Shell) exitEditor(destination tui.Widget) {
	if s == nil {
		return
	}
	s.cancelFocusRequest()
	s.editor = editorInteraction{phase: editorNormal, channel: s.activeChannel()}
	s.composerOverlay = nil
	if s.mv != nil {
		s.bindEditorState()
		if s.mv.composer != nil && s.cfg.Accessibility.VimNavigation {
			s.mv.composer.SetInputFocusEnabled(false)
		}
		s.mv.updateComposerStatus()
	}
	if destination != nil {
		s.queueFocus(destination)
	}
}

func (s *Shell) queueFocus(target tui.Widget) {
	if s == nil || target == nil {
		return
	}
	s.focusGeneration++
	s.focusRequest = tui.FocusRequest{Target: target, Generation: s.focusGeneration}
	s.focusRequestReady = true
	s.focusRequestActive = true
}

func (s *Shell) cancelFocusRequest() {
	if s == nil {
		return
	}
	s.focusGeneration++
	s.focusRequest = tui.FocusRequest{}
	s.focusRequestReady = false
	s.focusRequestActive = false
}

func (s *Shell) channelChanged(previous, current store.ChannelID) {
	if s == nil || previous == current {
		return
	}
	// A channel transition invalidates every exact composer request and every
	// suspended restore. The overlay may finish its own callback afterward, but
	// it can no longer resurrect input for the old channel.
	s.exitEditor(nil)
	s.composerOverlay = nil
}

func (s *Shell) sameFocusRequest(request tui.FocusRequest) bool {
	return s != nil && s.focusRequestActive && request.Generation == s.focusRequest.Generation && request.Target == s.focusRequest.Target
}

func (s *Shell) runLocalCommand(input string) bool {
	command, ok := parseLocalCommand(input)
	if !ok {
		return false
	}
	switch command.name {
	case "help":
		detail := ";help · ;quit · ;switch · ;settings · ;theme [name] · ;paste"
		if s.plugins != nil {
			if names := s.plugins.CommandNames(); len(names) > 0 {
				detail += " · plugins: ;" + strings.Join(names, " ;")
			}
		}
		s.ShowNotice("Local commands", detail)
	case "quit":
		s.Close()
		if s.cancel != nil {
			s.cancel()
		}
	case "switch":
		s.openQuickSwitcher()
		if qs, ok := s.overlay.(*QuickSwitcher); ok && len(command.args) > 0 {
			qs.input.SetValue(strings.Join(command.args, " "))
		}
	case "settings":
		guild := s.app.ActiveGuild()
		if guild == 0 || guild == app.DirectMessagesGuildID {
			s.ShowNotice("Settings unavailable", "Server settings are not available in direct messages")
		} else {
			s.openServerSettings(guild)
		}
	case "theme":
		s.runThemeCommand(command.args)
	case "paste", "img":
		s.pasteImage()
	default:
		if s.plugins != nil && s.plugins.RunCommand(command.name, command.args) {
			return true
		}
		s.ShowNotice("Unknown local command", "Use ;help to list local commands")
	}
	return true
}

// playVideo plays url in the full-screen media viewer. The region argument (the
// inline poster's rect) is ignored: true in-place inline playback is not
// feasible with mpv — it clears its whole terminal each frame and its output is
// not coordinate-translated when forwarded — so playback runs full-screen in an
// overlay, where mpv's black backdrop reads as the intended player. The inline
// poster + ▶ stays as the trigger.
func (s *Shell) playVideo(url string, _ media.Rect) {
	if !s.mediaCfg.VideoEnabled {
		s.ShowNotice("Video", "Video playback is disabled in media/privacy settings")
		return
	}
	if s.video == nil || !s.video.Available() {
		s.ShowNotice("Video", "Install mpv to play videos inline")
		return
	}
	sz, err := term.ProbeSize()
	if err != nil {
		s.ShowToast("Video error", err)
		return
	}
	full := videoRectForSize(sz.Width, sz.Height)
	v := newMediaViewer(s.styles, "▶ playing", url, nil, nil, s.closeOverlay)
	v.setVideoControls(
		func() {
			if err := s.video.TogglePause(); err != nil {
				s.ShowToast("Video control", err)
			}
		},
		func() {
			if err := s.video.Replay(); err != nil {
				s.ShowToast("Video control", err)
			}
		},
		func(percent float64) {
			if err := s.video.SeekPercent(percent); err != nil {
				s.ShowToast("Video control", err)
			}
		},
		s.video.Status,
	)
	v.setVideoKeys(
		s.cfg.Keys.VideoPause,
		s.cfg.Keys.VideoSeekBackward,
		s.cfg.Keys.VideoSeekForward,
		s.cfg.Keys.VideoReplay,
		func(seconds float64) {
			if err := s.video.SeekRelative(seconds); err != nil {
				s.ShowToast("Video control", err)
			}
		},
	)
	v.width, v.height = sz.Width, sz.Height
	v.setVideoResize(func(width, height int) {
		s.app.Post(func() {
			if s.overlay != v || s.video == nil {
				return
			}
			region := videoRectForSize(width, height)
			if err := s.video.Resize(region); err != nil {
				s.ShowToast("Video resize", err)
				return
			}
			s.videoRegion = region
		})
	})
	s.setIndependentOverlay(v)
	s.videoRegion = full
	onExit := func() { s.app.Post(s.closeOverlay) }
	if err := s.video.Play(url, full, s.app.WriteRaw, onExit); err != nil {
		s.overlay = nil
		s.composerOverlay = nil
		s.videoRegion = media.Rect{}
		s.ShowToast("Video error", err)
		return
	}
	s.app.Invalidate()
}

func videoRectForSize(width, height int) media.Rect {
	return media.Rect{
		X:    mediaViewerPadding,
		Y:    mediaViewerPadding,
		Cols: max(width-mediaViewerPadding*2, 1),
		Rows: max(height-videoControlRows-mediaViewerPadding*2, 1),
	}
}

// openMediaViewer shows an already-loaded image or GIF frame enlarged in the
// full-screen viewer.
func (s *Shell) openMediaViewer(url string, img image.Image, frames []media.Frame) {
	v := newMediaViewer(s.styles, "Esc to close", url, img, frames, s.closeOverlay)
	s.setIndependentOverlay(v)
	s.app.Invalidate()

	// Inline media is deliberately downscaled before caching. The full-screen
	// viewer refetches from the raw disk cache (or network) with no pixel cap so
	// enlargement does not magnify a thumbnail.
	if s.viewerCancel != nil {
		s.viewerCancel()
	}
	ctx, cancel := context.WithCancel(s.lifecycleCtx)
	s.viewerCancel = cancel
	fetcher := newViewerMediaFetcher(viewerMediaConfig(s.cfg))
	if fetcher == nil {
		return
	}
	animated := len(frames) > 1 || media.ClassifyURL(url) == media.ClassGIF
	s.lifecycleWG.Add(1)
	go func() {
		defer s.lifecycleWG.Done()
		if animated {
			fullFrames, err := fetcher.FetchGIF(ctx, url)
			if err != nil || len(fullFrames) == 0 {
				return
			}
			s.app.Post(func() {
				if s.overlay == v {
					v.setFrames(fullFrames)
				}
			})
			return
		}
		full, err := fetcher.Fetch(ctx, url)
		if err != nil {
			return
		}
		s.app.Post(func() {
			if s.overlay == v {
				v.setImage(full)
			}
		})
	}()
}

// teardownVideo stops the current playback and erases mpv's final frame from the
// screen (mpv leaves it behind because it runs with alt-screen off).
func (s *Shell) teardownVideo() {
	if s.video == nil {
		return
	}
	s.video.Stop()
	if !s.videoRegion.Empty() {
		// mpv owns an untracked Kitty image. Delete it after stopping; closeOverlay
		// requests a force repaint, and the event loop drains this raw command
		// before restoring the widget tree.
		s.app.WriteRaw(media.KittyDeleteAllImages())
		s.videoRegion = media.Rect{}
	}
}

// pasteImage attaches a clipboard image, reporting an empty/text-only clipboard
// as a toast. It is the explicit trigger (ctrl+v / ;paste).
func (s *Shell) pasteImage() { s.tryPasteImage(false) }

// tryPasteImage reads an image from the system clipboard, writes it to a
// temporary file, and stages it as a composer attachment, then opens a preview.
// Text paste is unaffected: this only touches the clipboard's image data. When
// quiet is true an empty/text-only clipboard is a silent no-op (used for the
// bracketed-paste hook); otherwise it surfaces as a toast. It reports whether an
// image was attached.
func (s *Shell) tryPasteImage(quiet bool) bool {
	if s == nil || !s.cfg.Privacy.ClipboardImages {
		if !quiet && s != nil {
			s.ShowTimedNotice("Paste image", "Clipboard image access is disabled in [privacy]", pasteNoticeTTL)
		}
		return false
	}
	s.clipboardMu.Lock()
	if s.clipboardBusy {
		s.clipboardMu.Unlock()
		return true
	}
	s.clipboardMu.Unlock()
	timeout := time.Duration(s.cfg.Privacy.ClipboardTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	maxBytes := s.cfg.Privacy.ClipboardMaxBytes
	if maxBytes <= 0 {
		maxBytes = 25 << 20
	}
	ctx, cancel := context.WithTimeout(s.lifecycleCtx, timeout)
	s.clipboardMu.Lock()
	// This second check makes the state transition safe even if a non-UI test
	// seam invokes paste concurrently. Production calls remain UI-goroutine owned.
	if s.clipboardBusy {
		s.clipboardMu.Unlock()
		cancel()
		return true
	}
	s.clipboardCancel = cancel
	s.clipboardBusy = true
	s.clipboardMu.Unlock()
	s.lifecycleWG.Add(1)
	go func() {
		defer s.lifecycleWG.Done()
		data, ext, err := term.ReadClipboardImageContext(ctx, maxBytes)
		var tempPath string
		if err == nil {
			f, createErr := os.CreateTemp("", "tuicord-paste-*."+ext)
			if createErr != nil {
				err = createErr
			} else {
				tempPath = f.Name()
				if _, writeErr := f.Write(data); writeErr != nil {
					err = writeErr
				}
				if closeErr := f.Close(); err == nil && closeErr != nil {
					err = closeErr
				}
			}
		}
		s.finishClipboardPaste(ctx, cancel, quiet, data, ext, tempPath, err)
	}()
	return true
}

// finishClipboardPaste transfers temp-file ownership to the UI only while both
// the operation and Shell are live. TryPost closes the final race where the
// event loop exits between the worker's lifecycle check and queue insertion.
func (s *Shell) finishClipboardPaste(ctx context.Context, cancel context.CancelFunc, quiet bool, data []byte, ext, tempPath string, resultErr error) {
	cleanup := func() {
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
	}
	clearBusy := func() {
		s.clipboardMu.Lock()
		s.clipboardBusy = false
		s.clipboardCancel = nil
		s.clipboardMu.Unlock()
	}
	if s == nil {
		cleanup()
		cancel()
		return
	}
	// An operation deadline is a user-visible completion, not a lifecycle exit.
	// It must reach the UI so the busy state clears and the timeout is reported.
	// Shell cancellation remains silent and does not post into a stopped loop.
	if s.lifecycleCtx == nil || s.lifecycleCtx.Err() != nil {
		cleanup()
		clearBusy()
		cancel()
		return
	}
	tryPost := s.tryPost
	if tryPost == nil && s.app != nil {
		tryPost = s.app.TryPost
	}
	if tryPost == nil || !tryPost(func() {
		// Shutdown can begin after TryPost accepted this closure. Recheck before
		// staging, otherwise the temp file would have no UI owner to delete it.
		live := s.lifecycleCtx != nil && s.lifecycleCtx.Err() == nil
		if ctxErr := ctx.Err(); ctxErr != nil {
			switch {
			case errors.Is(ctxErr, context.DeadlineExceeded):
				resultErr = fmt.Errorf("clipboard image extraction timed out: %w", ctxErr)
			case resultErr == nil:
				resultErr = fmt.Errorf("clipboard image extraction canceled: %w", ctxErr)
			}
		}
		clearBusy()
		cancel()
		if !live {
			cleanup()
			return
		}
		if resultErr != nil {
			cleanup()
			if quiet && errors.Is(resultErr, term.ErrNoClipboardImage) {
				return
			}
			s.ShowTimedNotice("Paste image", resultErr.Error(), pasteNoticeTTL)
			return
		}
		filename := fmt.Sprintf("pasted-%d.%s", time.Now().Unix(), ext)
		if err := s.mv.StageTempImage(tempPath, filename, int64(len(data))); err != nil {
			cleanup()
			s.ShowTimedNotice("Paste image", err.Error(), pasteNoticeTTL)
			return
		}
		s.ShowTimedNotice("Image attached", filename+" ("+formatAttachmentSize(int64(len(data)))+") · press Enter to send", pasteNoticeTTL)
	}) {
		// TryPost only rejects while the event loop is stopping. Clearing under
		// clipboardMu avoids racing Close and guarantees no stale busy/cancel state.
		cleanup()
		clearBusy()
		cancel()
	}
}

// pasteNoticeTTL is how long paste confirmations stay before auto-dismissing.
const pasteNoticeTTL = 2 * time.Second

// runThemeCommand applies a plugin-registered theme by name, or lists the
// available themes when called without an argument.
func (s *Shell) runThemeCommand(args []string) {
	if s.plugins == nil {
		s.ShowNotice("Themes", "No plugins are loaded")
		return
	}
	names := s.plugins.ThemeNames()
	if len(args) == 0 {
		if len(names) == 0 {
			s.ShowNotice("Themes", "No themes registered. Plugins add them with tuicord.theme(name, palette).")
			return
		}
		s.ShowNotice("Themes", "Available: "+strings.Join(names, ", ")+" · use ;theme <name>")
		return
	}
	name := args[0]
	if s.plugins.ApplyTheme(name) {
		s.ShowNotice("Theme", "Applied "+name)
	} else {
		s.ShowNotice("Unknown theme", "Registered: "+strings.Join(names, ", "))
	}
}

func (s *Shell) openForumPostPrompt(title string) {
	forum, ok := s.app.Store().Channel(s.mv.forumID)
	if !ok || forum.Forum == nil {
		return
	}
	p := NewForumPostPrompt(forum.Forum.Tags, s.styles, func(title, body string, tags []uint64) {
		s.app.CreateForumPost(forum.ID, title, body, tags)
	}, s.closeOverlay)
	p.SetTitle(title)
	s.setIndependentOverlay(p)
}

func (s *Shell) openForumFilterMenu() {
	if s.mv.forumView == nil {
		return
	}
	forum, ok := s.app.Store().Channel(s.mv.forumID)
	if !ok || forum.Forum == nil {
		return
	}
	items := []widget.MenuItem{{Label: "All tags", OnSelect: func() { s.closePopup(); s.mv.forumView.SetFilter(0) }}}
	for _, value := range forum.Forum.Tags {
		tag := value
		items = append(items, widget.MenuItem{Label: tag.Name, OnSelect: func() { s.closePopup(); s.mv.forumView.SetFilter(tag.ID) }})
	}
	s.showPopupMenu(items, 0, 0)
}

func (s *Shell) setIndependentOverlay(overlay tui.Widget) {
	if s == nil {
		return
	}
	if s.cfg.Accessibility.VimNavigation && s.editor.phase != editorNormal {
		s.exitEditor(nil)
	} else {
		s.cancelFocusRequest()
	}
	s.composerOverlay = nil
	s.overlay = overlay
}

func (s *Shell) setComposerOverlay(overlay tui.Widget) {
	if s == nil {
		return
	}
	s.composerOverlay = overlay
	if s.cfg.Accessibility.VimNavigation && (s.editor.phase == editorInput || s.editor.phase == editorFocusPending) {
		s.cancelFocusRequest()
		s.editor.phase = editorOverlaySuspended
		s.editor.allowReplacement = false
		if s.mv != nil && s.mv.composer != nil {
			s.mv.composer.SetInputFocusEnabled(false)
			s.mv.updateComposerStatus()
		}
	} else {
		s.cancelFocusRequest()
	}
	s.overlay = overlay
}

func (s *Shell) current() tui.Widget {
	if s.overlay != nil {
		return s.overlay
	}
	return s.mv.Root
}

// Children exposes the active subtree.
func (s *Shell) Children() []tui.Widget { return []tui.Widget{s.current()} }

// Measure delegates to the active subtree.
func (s *Shell) Measure(avail tui.Size) tui.Size { return s.current().Measure(avail) }

// Layout returns the shell node wrapping the active subtree.
func (s *Shell) Layout() *layout.Node {
	s.node.Children = []*layout.Node{s.current().Layout()}
	return &s.node
}

// TakeFocusRequest transfers a generation-scoped exact target after the
// current event finishes routing. Taking does not make it valid forever: the
// runtime rechecks FocusRequestValid before a deferred render-time attempt.
func (s *Shell) TakeFocusRequest() (tui.FocusRequest, bool) {
	if s == nil || !s.focusRequestReady {
		return tui.FocusRequest{}, false
	}
	request := s.focusRequest
	s.focusRequestReady = false
	return request, true
}

func (s *Shell) FocusRequestValid(request tui.FocusRequest) bool {
	if !s.sameFocusRequest(request) {
		return false
	}
	if s.mv == nil || request.Target != s.mv.composer {
		return true
	}
	if !s.composerWritable() || s.overlay != nil {
		return false
	}
	if !s.cfg.Accessibility.VimNavigation {
		return true
	}
	return s.editor.phase == editorFocusPending && s.editor.channel == s.activeChannel()
}

func (s *Shell) FocusRequestDone(request tui.FocusRequest, applied bool) {
	if !s.sameFocusRequest(request) {
		return
	}
	s.focusRequestActive = false
	s.focusRequestReady = false
	s.focusRequest = tui.FocusRequest{}
	if applied && s.mv != nil && request.Target == s.mv.composer && s.editor.phase == editorFocusPending {
		// Setting an already focused owner is successful but emits no focus-change
		// transition. Complete the pending->input state here as the equivalent
		// acknowledgement.
		s.editor.phase = editorInput
		s.editor.allowReplacement = false
		s.mv.updateComposerStatus()
		return
	}
	if !applied && s.editor.phase == editorFocusPending {
		s.exitEditor(nil)
	}
}

func (s *Shell) TakeFocusTraversalRequest() int {
	if s == nil {
		return 0
	}
	step := s.focusTraversal
	s.focusTraversal = 0
	return step
}

// FocusChanged enforces the editor invariant for every runtime focus path:
// outside a composer-owned overlay, Vim input means exact composer focus.
func (s *Shell) FocusChanged(change tui.FocusChange) {
	if s == nil || s.mv == nil || s.mv.composer == nil {
		return
	}
	if !s.cfg.Accessibility.VimNavigation {
		// Exact non-Vim requests can also be deferred while the composer is
		// hidden. Any explicit focus move supersedes that stale intent.
		if s.focusRequestActive && change.Current != s.focusRequest.Target {
			s.cancelFocusRequest()
		}
		return
	}
	composerFocused := change.Current == s.mv.composer
	switch s.editor.phase {
	case editorFocusPending:
		if composerFocused {
			if !s.composerWritable() || s.overlay != nil || s.editor.channel != s.activeChannel() {
				s.exitEditor(nil)
				return
			}
			s.editor.phase = editorInput
			s.editor.allowReplacement = false
			s.mv.updateComposerStatus()
			return
		}
		if change.Reason == tui.FocusChangeReplace && s.editor.allowReplacement {
			s.editor.allowReplacement = false
			return
		}
		s.exitEditor(nil)
	case editorInput:
		if !composerFocused {
			s.exitEditor(nil)
		}
	case editorOverlaySuspended:
		// The overlay owns focus until closeOverlay explicitly restores input.
	case editorNormal:
		if s.focusRequestActive && change.Current != s.focusRequest.Target {
			s.cancelFocusRequest()
		}
	}
}

// Draw is a no-op; children draw themselves.
func (s *Shell) Draw(screen.Region) {}

func (s *Shell) DrawOverlay(r screen.Region) {
	if s != nil && s.popup != nil {
		s.popup.Measure(tui.Size{W: r.Width(), H: r.Height()})
		s.popup.Draw(r)
	}
	if s != nil {
		for _, toast := range s.toasts {
			if toast != nil {
				toast.bounds = screen.Rect{}
			}
		}
		bottom := r.Height() - 1
		for _, toast := range s.toasts {
			if bottom < 0 {
				break
			}
			width := min(toastWidth, r.Width())
			height := toast.height(width, bottom+1)
			if height <= 0 {
				continue
			}
			x := max(r.Width()-width-1, 0)
			y := bottom - height + 1
			toast.drawAt(r, x, y, width, height)
			bottom = y - 1
		}
	}
}

// Handle routes global shortcuts and overlay dismissal, delegating everything
// else to the active subtree.
// Animating reports whether the chat has a visible inline animation (a GIF),
// letting the runtime raise the tick cadence only while one is on screen.
func (s *Shell) Animating() bool {
	if s == nil {
		return false
	}
	if animator, ok := s.overlay.(tui.Animator); ok {
		return animator.Animating()
	}
	return s.mv != nil && s.mv.chat.Animating()
}

// HandleOverlay routes pointer input in reverse draw order: toasts are painted
// above popup menus, and popup menus are painted above the retained tree.
func (s *Shell) HandleOverlay(ev tui.Event) bool {
	if s == nil {
		return false
	}
	if mouse, ok := ev.(input.MouseEvent); ok {
		if s.handleToastPointer(mouse) {
			return true
		}
		if _, ok := s.popup.(*pluginViewport); ok {
			// tui.OverlayHitTester dispatches viewport pointer input directly so
			// its component-owned drag/resize handles win without Shell geometry.
			return false
		}
		return s.popup != nil && s.popup.Handle(mouse)
	}
	if s.popup != nil {
		if s.popup.Handle(ev) {
			return true
		}
		// Plugin viewports are non-modal status surfaces. If they do not handle
		// an event, continue through Shell's normal/global routing so Vim mode
		// can still enter INSERT while the panel is visible. Other popups keep
		// their modal event barrier.
		if _, ok := s.popup.(*pluginViewport); !ok {
			return false
		}
	}
	key, isKey := ev.(input.KeyEvent)
	if !isKey || key.Release {
		return false
	}
	// Esc is a root-level modal transition. Claim it before a focused chat or
	// list can consume it, while still letting a topmost popup handle it first.
	if keyMatches(key, s.cfg.Keys.Vim.ExitInput) || key.Key == input.KeyEsc && s.cfg.Keys.Vim.ExitInput == "" {
		if s.overlay != nil {
			s.closeOverlay()
			return true
		}
		switch s.editor.phase {
		case editorInput:
			s.leaveInputMode()
			return true
		case editorFocusPending:
			s.exitEditor(nil)
			return true
		}
		if s.mv != nil && s.mv.CancelComposerMode() {
			return true
		}
	}
	if s.overlay != nil {
		return false
	}
	// Configured focus claims are global on the main surface. Handling them here
	// prevents a plain-rune binding from being inserted into the composer or
	// consumed as a chat Vim motion. Overlay-local widgets still get first claim
	// because this path is disabled while an overlay is active.
	if keyMatches(key, s.cfg.Keys.NextPanel) {
		s.focusTraversal++
		return true
	}
	if keyMatches(key, s.cfg.Keys.FocusComposer) && (!s.cfg.Accessibility.VimNavigation || key.Key != input.KeyEsc) && s.composerWritable() {
		if !s.cfg.Accessibility.VimNavigation || s.editor.phase == editorNormal {
			s.focusComposer()
		}
		return true
	}
	return false
}

// OverlayAt returns a component-owned floating hit target. Shell manages only
// z-order; each component owns its bounds and interaction geometry.
func (s *Shell) OverlayAt(x, y int) tui.Widget {
	if hit, ok := s.popup.(tui.OverlayHit); ok && hit.OverlayHit(x, y) {
		return s.popup
	}
	return nil
}

func (s *Shell) handleToastPointer(mouse input.MouseEvent) bool {
	if s == nil {
		return false
	}
	for i := len(s.toasts) - 1; i >= 0; i-- {
		toast := s.toasts[i]
		if toast == nil || !toast.contains(mouse.X, mouse.Y) {
			continue
		}
		handled := toast.Handle(mouse)
		if handled && toast.wantsDismiss(mouse) {
			s.toasts = append(s.toasts[:i], s.toasts[i+1:]...)
		}
		// Even a non-actionable toast is an opaque drawn surface. Never let its
		// pointer input click or focus a covered popup/tree widget.
		return true
	}
	return false
}

func (s *Shell) Handle(ev tui.Event) bool {
	if focus, ok := ev.(input.FocusEvent); ok {
		s.unfocused = !focus.Focused
		return true
	}
	if _, tick := ev.(input.TickEvent); tick && s.expireToasts(s.clock()) {
		return true
	}
	if _, idle := ev.(input.TickEvent); idle {
		if s.prefetch != nil && !s.prefetchIdle && prefetchEligible(s.lastActivity, time.Now()) {
			s.prefetchIdle = true
			s.prefetch.Start(mediaPrefetchURLs(s.app.Store(), s.app.ActiveGuild()))
		}
	} else if userActivity(ev) {
		s.prefetchIdle = false
		s.lastActivity = time.Now()
		if s.prefetch != nil {
			s.prefetch.Stop()
		}
	}
	key, isKey := ev.(input.KeyEvent)

	if mouse, ok := ev.(input.MouseEvent); ok {
		if s.handleToastPointer(mouse) {
			return true
		}
	} else if len(s.toasts) > 0 && s.toasts[0].Handle(ev) {
		if s.toasts[0].wantsDismiss(ev) {
			s.toasts = s.toasts[1:]
		}
		return true
	}

	if s.popup != nil {
		if s.popup.Handle(ev) {
			return true
		}
		// Plugin viewports are non-modal status surfaces. If they do not handle
		// an event, continue through Shell's normal/global routing so Vim mode
		// can still enter INSERT while the panel is visible. Other popups keep
		// their modal event barrier.
		if _, ok := s.popup.(*pluginViewport); !ok {
			return false
		}
	}
	if mouse, ok := ev.(input.MouseEvent); ok && mouse.Kind == input.MousePress {
		s.forumPreview = nil
	}

	if s.overlay != nil {
		// Focused overlay descendants already handled and bubbled this event.
		// Shell owns only global dismissal here; redispatching through the whole
		// overlay tree would reach unrelated hidden inputs.
		if isKey && (keyMatches(key, s.cfg.Keys.Help) || key.Key == input.KeyEsc) {
			s.closeOverlay()
			return true
		}
		return false
	}

	// A bracketed paste carrying no text is what terminals emit when the
	// clipboard holds an image and the user hits their native paste bind (e.g.
	// ctrl+shift+v): the text target is empty. Treat it as an image paste so the
	// default paste shortcut attaches images too. A real image-less empty paste
	// is a harmless no-op.
	if paste, ok := ev.(input.PasteEvent); ok && strings.TrimSpace(paste.Text) == "" {
		if s.tryPasteImage(true) {
			return true
		}
	}

	if mouse, ok := ev.(input.MouseEvent); ok && mouse.Kind == input.MousePress && mouse.Btn == input.ButtonRight {
		if msg, ok := s.mv.chat.TakeContextMessage(); ok {
			s.openMessageMenu(msg, mouse.X, mouse.Y)
			return true
		}
		if row, ok := s.mv.ChannelContext(); ok {
			s.openChannelMenu(row, mouse.X, mouse.Y)
			return true
		}
		if row, ok := s.mv.GuildContext(); ok {
			s.openGuildMenu(row, mouse.X, mouse.Y)
			return true
		}
	}

	if isKey {
		switch {
		case s.cfg.Accessibility.VimNavigation && keyMatches(key, s.cfg.Keys.Vim.ExitInput) && s.editor.phase == editorInput:
			s.leaveInputMode()
			return true
		case s.cfg.Accessibility.VimNavigation && keyMatches(key, s.cfg.Keys.Vim.ExitInput) && s.editor.phase == editorFocusPending:
			s.exitEditor(nil)
			return true
		case s.cfg.Accessibility.VimNavigation && s.editor.phase == editorNormal && keyMatches(key, s.cfg.Keys.Vim.Insert) && s.composerWritable():
			return s.beginComposerInput(false)
		case key.Key == input.KeyRune && key.Rune == '+' && key.Mods == 0:
			s.openHotSwitch()
			return true
		case key.Key == input.KeyEsc && s.mv.CancelComposerMode():
			return true
		case keyMatches(key, s.cfg.Keys.NextPanel):
			s.focusTraversal++
			return true
		case keyMatches(key, s.cfg.Keys.FocusComposer) && (!s.cfg.Accessibility.VimNavigation || key.Key != input.KeyEsc) && s.composerWritable():
			s.focusComposer()
			return true
		case keyMatches(key, s.cfg.Keys.QuickSwitcher):
			s.openQuickSwitcher()
			return true
		case keyMatches(key, s.cfg.Keys.Picker):
			s.openPicker()
			return true
		case s.cfg.Keys.PasteImage != "" && keyMatches(key, s.cfg.Keys.PasteImage):
			s.pasteImage()
			return true
		case keyMatches(key, s.cfg.Keys.Help):
			s.setIndependentOverlay(NewHelpOverlay(s.cfg))
			return true
		}
	}
	handled := false
	if _, tick := ev.(input.TickEvent); tick && s.mv != nil && s.mv.Root != nil {
		handled = s.mv.Root.Handle(ev)
	}
	if action, ok := s.mv.chat.TakeEntityAction(); ok {
		s.dispatchEntityAction(action)
		return true
	}
	if action, ok := s.mv.chat.TakeComponentAction(); ok {
		s.dispatchComponentAction(action)
		return true
	}
	// Plugin key bindings are a fallback: they fire only for keys no built-in
	// binding or focused widget (including the composer) consumed, so a plugin
	// cannot shadow core navigation or intercept text input.
	if !handled && isKey && !key.Release && s.plugins != nil {
		for _, spec := range s.plugins.KeySpecs() {
			if keyMatches(key, spec) && s.plugins.RunKey(spec) {
				return true
			}
		}
	}
	return handled
}

const idlePrefetchDelay = 2 * time.Second

func prefetchEligible(lastActivity, now time.Time) bool {
	return !lastActivity.IsZero() && !now.Before(lastActivity) && now.Sub(lastActivity) >= idlePrefetchDelay
}

func userActivity(ev tui.Event) bool {
	switch ev := ev.(type) {
	case input.KeyEvent:
		return !ev.Release
	case input.MouseEvent:
		return ev.Kind == input.MousePress
	case input.PasteEvent:
		return ev.Text != ""
	default:
		return false
	}
}

func (s *Shell) setForumHover(id store.ChannelID, isForum bool) {
	// Forum previews are rendered in the retained right-hand split pane owned
	// by MainView; pointer hover no longer creates a screen-wide overlay.
	_ = id
	_ = isForum
	s.forumPreview = nil
}

type forumPreview struct {
	title  string
	labels []string
	style  Styles
}

func (p *forumPreview) Draw(r screen.Region) {
	if p == nil || r.Width() < 20 || r.Height() < 4 {
		return
	}
	w := min(52, max(28, r.Width()/3))
	h := min(r.Height()-2, len(p.labels)+3)
	x := r.Width() - w - 1
	box := r.Clip(screen.Rect{X: x, Y: 1, W: w, H: h})
	bg := screen.RGB(28, 31, 38)
	border := p.style.Accent
	box.Fill(screen.Rect{W: w, H: h}, screen.Cell{Content: " ", Style: screen.Style{Bg: bg}})
	for xx := 0; xx < w; xx++ {
		box.Set(xx, 0, screen.Cell{Content: "─", Style: border})
		box.Set(xx, h-1, screen.Cell{Content: "─", Style: border})
	}
	for yy := 0; yy < h; yy++ {
		box.Set(0, yy, screen.Cell{Content: "│", Style: border})
		box.Set(w-1, yy, screen.Cell{Content: "│", Style: border})
	}
	box.Set(0, 0, screen.Cell{Content: "╭", Style: border})
	box.Set(w-1, 0, screen.Cell{Content: "╮", Style: border})
	box.Set(0, h-1, screen.Cell{Content: "╰", Style: border})
	box.Set(w-1, h-1, screen.Cell{Content: "╯", Style: border})
	drawPreviewText(box, 2, 1, p.title+" · forum", w-4, screen.Style{Fg: p.style.Accent.Fg, Bg: bg, Attrs: screen.Bold})
	for i, label := range p.labels {
		if i+2 >= h-1 {
			break
		}
		drawPreviewText(box, 2, i+2, "· "+label, w-4, screen.Style{Fg: p.style.Text.Fg, Bg: bg})
	}
}

func drawPreviewText(r screen.Region, x, y int, value string, width int, style screen.Style) {
	col := x
	for cluster := range text.Clusters(value) {
		if cluster.Width == 0 || col-x+cluster.Width > width {
			break
		}
		r.Set(col, y, screen.Cell{Content: cluster.Text, Style: style})
		col += cluster.Width
	}
}

func (s *Shell) dispatchEntityAction(action markup.Action) {
	if action.Kind == markup.ActionOpenURL {
		if err := term.OpenURL(action.Target); err != nil {
			s.ShowToast("Open link", err)
		}
		return
	}
	id, err := strconv.ParseUint(action.Target, 10, 64)
	if err != nil {
		return
	}
	switch action.Kind {
	case markup.ActionUserMention:
		s.openProfile(store.UserID(id))
	case markup.ActionRoleMention:
		s.openRoleOptions(store.RoleID(id))
	}
}

// openProfile uses the gateway member cache as the reliable offline fallback.
// The card shows role-colored role chips, the profile picture (fetched
// asynchronously), and the servers the local caches know are shared.
func (s *Shell) openProfile(id store.UserID) {
	s.openProfileWithFallback(id, store.Member{})
}

// openMessageAuthorProfile preserves the identity carried by the selected
// message. Message authors are not guaranteed to be present in the guild or DM
// member caches, but that cache miss must not turn Vim's u action into a notice.
func (s *Shell) openMessageAuthorProfile(msg store.Message) {
	s.openProfileWithFallback(msg.AuthorID, store.Member{
		ID:        msg.AuthorID,
		Name:      msg.Author,
		AvatarURL: msg.AuthorAvatarURL,
	})
}

func (s *Shell) openProfileWithFallback(id store.UserID, fallback store.Member) {
	st := s.app.Store()
	guild := s.app.ActiveGuild()
	details := buildProfileDetails(st, guild, 0, id)
	if m, ok := memberForContext(st, guild, s.app.ActiveChannel(), id); ok {
		details = fillProfileIdentity(details, m)
	}
	details = fillProfileIdentity(details, fallback)
	if details.Name == "" && details.Username == "" {
		s.ShowNotice("Profile", "User "+strconv.FormatUint(uint64(id), 10))
		return
	}
	popup := NewProfilePopup(details, s.styles, func(dm store.ChannelID) {
		s.closePopup()
		s.mv.NavigateToChannel(dm)
	}, s.closePopup)
	s.setPopup(popup)
	s.fetchProfileAvatar(popup, details.AvatarURL)
	// Roles are absent from message/history payloads, so fetch the full guild
	// member on demand and refresh the still-open card once it resolves.
	s.app.EnsureMemberDetail(guild, id, func() {
		if s.popup != popup {
			return
		}
		refreshed := buildProfileDetails(s.app.Store(), guild, 0, id)
		popup.SetDetails(fillProfileIdentity(refreshed, fallback))
		s.app.Invalidate()
	})
}

func fillProfileIdentity(details profileDetails, fallback store.Member) profileDetails {
	if details.Name == "" {
		details.Name = fallback.Name
	}
	if details.Username == "" {
		details.Username = fallback.Username
	}
	if details.Nick == "" {
		details.Nick = fallback.Nick
	}
	if details.AvatarURL == "" {
		details.AvatarURL = fallback.AvatarURL
	}
	return details
}

// fetchProfileAvatar resolves a profile card's picture off the UI goroutine and
// attaches it if the card is still open when the image arrives.
func (s *Shell) fetchProfileAvatar(popup *ProfilePopup, url string) {
	if url == "" || !s.mediaCfg.Enabled {
		return
	}
	fetcher := s.profileFetcher
	if fetcher == nil {
		if s.prefetch != nil {
			fetcher = s.prefetch.fetcher
		} else {
			fetcher = newChatMediaFetcher(s.mediaCfg)
		}
		s.profileFetcher = fetcher
	}
	if fetcher == nil {
		return
	}
	ctx := s.lifecycleCtx
	s.lifecycleWG.Add(1)
	go func() {
		defer s.lifecycleWG.Done()
		img, err := fetcher.Fetch(ctx, url)
		if err != nil || img == nil {
			return
		}
		s.app.Post(func() {
			if s.popup == popup {
				popup.SetAvatar(img)
				s.app.Invalidate()
			}
		})
	}()
}

func (s *Shell) openRoleOptions(id store.RoleID) {
	role, ok := s.app.Store().Role(s.app.ActiveGuild(), id)
	if !ok {
		return
	}
	canManage := s.app.Store().MemberCan(s.app.ActiveGuild(), s.app.SelfID(), store.PermManageRoles)
	items := []widget.MenuItem{
		{Label: fmt.Sprintf("%s · #%06X", role.Name, role.Color), Disabled: true},
		{Label: "Create role…", Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.setIndependentOverlay(NewPrompt("Create role", "Role name…", s.styles, func(name string) { s.app.CreateRole(s.app.ActiveGuild(), name) }, s.closeOverlay))
		}},
		{Label: "Rename…", Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.setIndependentOverlay(NewPrompt("Rename role", "Role name…", s.styles, func(name string) { s.app.RenameRole(s.app.ActiveGuild(), id, name) }, s.closeOverlay))
		}},
		{Label: "Change color…", Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.setIndependentOverlay(NewPrompt("Role color", "RRGGBB", s.styles, func(value string) {
				if color, err := strconv.ParseUint(strings.TrimPrefix(value, "#"), 16, 32); err == nil && color <= 0xffffff {
					s.app.SetRoleColor(s.app.ActiveGuild(), id, uint32(color))
				} else {
					s.ShowNotice("Invalid color", "Use six hexadecimal digits")
				}
			}, s.closeOverlay))
		}},
		{Label: "Toggle hoist", Disabled: !canManage, OnSelect: func() { s.closePopup(); s.app.SetRoleHoist(s.app.ActiveGuild(), id, !role.Hoist) }},
		{Label: "Toggle mentionable", Disabled: !canManage, OnSelect: func() { s.closePopup(); s.app.SetRoleMentionable(s.app.ActiveGuild(), id, !role.Mentionable) }},
		{Separator: true},
		{Label: "Delete…", Danger: true, Disabled: !canManage, OnSelect: func() {
			s.showPopupMenu([]widget.MenuItem{{Label: "Delete — click again", Danger: true, OnSelect: func() { s.closePopup(); s.app.DeleteRole(s.app.ActiveGuild(), id) }}, {Label: "Cancel", OnSelect: s.closePopup}}, 0, 0)
		}},
	}
	s.showPopupMenu(items, 0, 0)
}

// dispatchComponentAction forwards a chat component activation to Discord.
// Link buttons have no interaction to submit; their URL goes to the clipboard.
func (s *Shell) dispatchComponentAction(action ComponentAction) {
	if action.Kind == store.ComponentLinkButton || (action.URL != "" && action.CustomID == "") {
		if err := term.CopyToClipboard(os.Stdout, action.URL); err != nil {
			s.ShowToast("Clipboard error", err)
			return
		}
		s.ShowNotice("Link copied", action.URL)
		return
	}
	switch action.Kind {
	case store.ComponentButton, store.ComponentSelect:
		s.app.SubmitComponent(app.ComponentSubmit{
			Message:       action.Message,
			ComponentType: action.RawType,
			CustomID:      action.CustomID,
			Values:        action.Values,
		})
	}
}

func (s *Shell) openQuickSwitcher() {
	s.setIndependentOverlay(NewQuickSwitcher(s.app.Store(), s.styles,
		func(guild store.GuildID, channel store.ChannelID) {
			s.mv.setActive(guild, channel)
			s.mv.RefreshChannels()
		},
		s.closeOverlay,
	))
}

// openHotSwitch opens the lightweight + picker from any focused panel.
func (s *Shell) openHotSwitch() {
	p := NewInlinePicker(s.app.Store(), s.styles, s.app.ActiveGuild(), s.app.ActiveChannel(), s.app.Store().HasNitro(), s.cfg.Nitro.Fake,
		'+', "", func(string) {}, nil, s.closeOverlay)
	p.SetSwitch(func(guild store.GuildID, channel store.ChannelID) {
		s.mv.setActive(guild, channel)
		s.mv.Refresh()
		if c, ok := s.app.Store().Channel(channel); ok && c.Kind != store.ChannelForum {
			s.app.LoadHistory(channel, 50)
		}
	})
	s.setIndependentOverlay(p)
}

// openPicker opens the emoji/sticker picker overlay over the composer. Chosen
// entries are inserted at the composer cursor.
func (s *Shell) openPicker() {
	st := s.app.Store()
	p := NewPicker(st, s.styles, s.app.ActiveGuild(), st.HasNitro(), s.cfg.Nitro.Fake,
		func(text string) { s.mv.InsertIntoComposer(text) },
		s.closeOverlay,
	)
	p.SetGIFSearch(s.app.SearchGIFs)
	p.SetRecentStickers(s.mv.recentStickers())
	p.SetFavorites(s.mv.favoriteEmojis(), s.mv.favoriteStickers(), s.mv.toggleFavorite)
	p.SetMedia(newChatMediaFetcher(s.mediaCfg), s.mediaCfg, s.app.Post)
	p.SetStickerSelect(func(id uint64) {
		s.app.SendSticker(id)
	})
	p.SetStickerRecent(s.mv.recordRecentSticker)
	s.setComposerOverlay(p)
}

// composerChanged opens or refreshes the relevant autocomplete menu when the
// token immediately before the cursor starts with a supported trigger.
func (s *Shell) composerChanged(value string, cursor int) {
	if s.completionSync {
		return
	}
	if s.cfg.Accessibility.VimNavigation && s.editor.phase != editorNormal && strings.HasSuffix(value, ";q") {
		s.completionSync = true
		s.mv.composer.SetValue(strings.TrimSuffix(value, ";q"))
		s.completionSync = false
		s.leaveInputMode()
		if _, inline := s.overlay.(*InlinePicker); inline {
			s.closeOverlay()
		}
		return
	}
	trigger, start, query, ok := completionToken(value, cursor)
	if !ok {
		switch s.overlay.(type) {
		case *InlinePicker, *CommandPicker, *LocalCommandPicker:
			s.closeOverlay()
		}
		return
	}
	if trigger == ';' {
		if _, picker := s.overlay.(*LocalCommandPicker); picker {
			return
		}
		s.openLocalCommandPicker(query, start)
		return
	}
	if trigger == '/' {
		if !s.cfg.Integrations.SlashCommands.Enabled {
			if _, picker := s.overlay.(*CommandPicker); picker {
				s.closeOverlay()
			}
			return
		}
		if _, picker := s.overlay.(*CommandPicker); picker {
			return
		}
		s.openCommandPicker(query)
		return
	}
	if _, inline := s.overlay.(*InlinePicker); inline {
		return
	}
	p := NewInlinePicker(s.app.Store(), s.styles, s.app.ActiveGuild(), s.app.ActiveChannel(), s.app.Store().HasNitro(), s.cfg.Nitro.Fake,
		trigger, query,
		func(insert string) {
			s.replaceCompletion(start, insert)
		},
		func(stickerID uint64) {
			s.app.SendSticker(stickerID)
		},
		s.closeOverlay,
	)
	p.SetFavorites(s.mv.favoriteEmojis(), s.mv.favoriteStickers())
	p.SetQueryChange(func(next string) {
		s.completionSync = true
		s.mv.ReplaceComposerRange(start, start+len(query)+1, string(trigger)+next)
		s.completionSync = false
		query = next
	})
	p.SetTriggerDelete(func() {
		s.completionSync = true
		s.mv.ReplaceComposerRange(start, start+1, "")
		s.completionSync = false
	})
	p.SetSwitch(func(guild store.GuildID, channel store.ChannelID) {
		s.replaceCompletion(start, "")
		s.mv.setActive(guild, channel)
		s.mv.Refresh()
		if c, ok := s.app.Store().Channel(channel); ok && c.Kind != store.ChannelForum {
			s.app.LoadHistory(channel, 50)
		}
	})
	p.SetMedia(newChatMediaFetcher(s.mediaCfg), s.mediaCfg, s.app.Post)
	s.setComposerOverlay(p)
}

func (s *Shell) openCommandPicker(query string) {
	if s.commandLoading || s.app.ActiveChannel() == 0 {
		return
	}
	s.commandLoading = true
	ctx := app.CommandContext{GuildID: s.app.ActiveGuild(), ChannelID: s.app.ActiveChannel()}
	go func() {
		commands, err := s.app.LoadCommands(ctx)
		s.app.Post(func() {
			s.commandLoading = false
			if err != nil {
				s.ShowNotice("Slash commands unavailable", "Could not load Discord application commands")
				return
			}
			if s.overlay != nil || s.app.ActiveGuild() != ctx.GuildID || s.app.ActiveChannel() != ctx.ChannelID || !strings.HasPrefix(s.mv.composer.Value(), "/") {
				return
			}
			if s.cfg.Accessibility.VimNavigation && s.editor.phase != editorInput {
				return
			}
			s.setComposerOverlay(NewCommandPicker(commands, s.styles, query, func(command app.ApplicationCommand) {
				if len(command.Options) != 0 {
					s.ShowNotice("Command options", "Guided command forms are not available yet")
					return
				}
				s.mv.composer.SetValue("")
				s.closeOverlay()
				s.app.SubmitCommand(command, nil)
			}, s.closeOverlay))
		})
	}()
}

func (s *Shell) openLocalCommandPicker(query string, start int) {
	if s == nil || s.mv == nil {
		return
	}
	var picker *LocalCommandPicker
	picker = NewLocalCommandPicker(s.localCommandSpecs(), s.styles, query, func(name string) {
		end := start + len(picker.Query()) + 1
		s.completionSync = true
		s.mv.ReplaceComposerRange(start, end, ";"+name+" ")
		s.completionSync = false
		s.closeOverlay()
	}, s.closeOverlay)
	s.setComposerOverlay(picker)
}

func (s *Shell) localCommandSpecs() []localCommandSpec {
	commands := []localCommandSpec{
		{Name: "help", Description: "Show local commands"},
		{Name: "quit", Description: "Exit Tuicord"},
		{Name: "switch", Description: "Switch channel"},
		{Name: "settings", Description: "Open server settings"},
		{Name: "theme", Description: "Change theme"},
		{Name: "paste", Description: "Paste an image"},
		{Name: "img", Description: "Paste an image"},
	}
	if s.plugins != nil {
		for _, name := range s.plugins.CommandNames() {
			commands = append(commands, localCommandSpec{Name: name, Description: "Plugin command"})
		}
	}
	return commands
}

func (s *Shell) replaceCompletion(start int, insert string) {
	if p, ok := s.overlay.(*InlinePicker); ok {
		startEnd := start + len(p.query) + 1
		s.completionSync = true
		s.mv.ReplaceComposerRange(start, startEnd, insert)
		s.completionSync = false
	}
}

// completionToken returns the current non-whitespace composer token when its
// first rune is one of the autocomplete triggers.
func completionToken(value string, cursor int) (rune, int, string, bool) {
	if cursor < 0 || cursor > len(value) {
		return 0, 0, "", false
	}
	start := cursor
	for start > 0 {
		prev := start - 1
		for prev > 0 && (value[prev]&0xc0) == 0x80 {
			prev--
		}
		r, _ := utf8.DecodeRuneInString(value[prev:start])
		if unicode.IsSpace(r) {
			break
		}
		start = prev
	}
	if start == cursor {
		return 0, 0, "", false
	}
	trigger, size := utf8.DecodeRuneInString(value[start:])
	if !strings.ContainsRune("/;:%#@&+", trigger) {
		return 0, 0, "", false
	}
	return trigger, start, value[start+size : cursor], true
}

func (s *Shell) openMessageMenu(msg store.Message, x, y int) {
	own := msg.AuthorID != 0 && msg.AuthorID == s.app.SelfID()
	canManage := s.canManageMessages(msg.ChannelID)
	canDelete := own || canManage
	deleteLabel := "Delete"
	if !own {
		deleteLabel = "Force delete"
	}
	pinLabel := "Pin"
	if msg.Pinned {
		pinLabel = "Unpin"
	}
	ch, _ := s.app.Store().Channel(msg.ChannelID)
	canThread := ch.Kind == store.ChannelText || ch.Kind == store.ChannelAnnouncement
	isAnnouncement := ch.Kind == store.ChannelAnnouncement
	items := []widget.MenuItem{
		{Label: "Reply", OnSelect: func() {
			s.closePopup()
			s.mv.BeginReply(msg, true)
			s.focusComposer()
		}},
		{Label: "Reply (no mention)", OnSelect: func() {
			s.closePopup()
			s.mv.BeginReply(msg, false)
			s.focusComposer()
		}},
		{Label: "Edit", Disabled: !own, OnSelect: func() {
			s.closePopup()
			s.mv.BeginEdit(msg)
			s.focusComposer()
		}},
		{Label: "Create thread…", Disabled: !canThread, OnSelect: func() {
			s.closePopup()
			s.openThreadPrompt(msg)
		}},
	}
	if isAnnouncement {
		items = append(items, widget.MenuItem{Label: "Publish", Disabled: !own, OnSelect: func() {
			s.closePopup()
			s.app.Publish(msg.ChannelID, msg.ID)
			s.ShowNotice("Publishing", "Message crossposted to followers")
		}})
	}
	items = append(items,
		widget.MenuItem{Separator: true},
		widget.MenuItem{Label: pinLabel, Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.app.SetPinned(msg.ChannelID, msg.ID, !msg.Pinned)
		}},
		widget.MenuItem{Label: deleteLabel, Danger: true, Disabled: !canDelete, OnSelect: func() {
			s.openDeleteMessageConfirm(msg, x, y, deleteLabel)
		}},
		widget.MenuItem{Separator: true},
		widget.MenuItem{Label: "Copy message ID", OnSelect: func() {
			s.closePopup()
			id := strconv.FormatUint(uint64(msg.ID), 10)
			if err := term.CopyToClipboard(os.Stdout, id); err != nil {
				s.ShowToast("Clipboard error", err)
				return
			}
			s.ShowNotice("Copied", "Message ID copied")
		}},
	)
	menu := widget.NewMenu(items)
	s.styleMenu(menu)
	menu.SetAnchor(x, y)
	menu.OnDismiss(s.closePopup)
	s.setPopup(menu)
}

func (s *Shell) openDeleteMessageConfirm(msg store.Message, x, y int, label string) {
	items := []widget.MenuItem{
		{Label: label + " — click again", Danger: true, OnSelect: func() {
			s.closePopup()
			s.app.DeleteMessage(msg.ChannelID, msg.ID)
		}},
		{Label: "Cancel", OnSelect: s.closePopup},
	}
	menu := widget.NewMenu(items)
	s.styleMenu(menu)
	menu.SetAnchor(x, y)
	menu.OnDismiss(s.closePopup)
	s.setPopup(menu)
}

// openChannelMenu shows the sidebar context menu for a channel or category:
// pin/unpin and copy id for channels, collapse/expand for categories.
func (s *Shell) openChannelMenu(row store.ChannelRow, x, y int) {
	if row.Thread {
		s.openThreadMenu(row, x, y)
		return
	}
	var items []widget.MenuItem
	if row.Category {
		label := "Collapse"
		if row.Collapsed {
			label = "Expand"
		}
		items = []widget.MenuItem{{Label: label, OnSelect: func() {
			s.closePopup()
			s.mv.ToggleCollapseCategory(row.ChannelID)
		}}}
	} else {
		pinLabel := "Pin channel"
		if s.mv.IsChannelPinned(row.ChannelID) {
			pinLabel = "Unpin channel"
		}
		items = []widget.MenuItem{
			{Label: pinLabel, OnSelect: func() {
				s.closePopup()
				s.mv.TogglePinChannel(row.ChannelID)
			}},
			{Separator: true},
			{Label: "Copy channel ID", OnSelect: func() {
				s.closePopup()
				s.copyID("Channel ID copied", uint64(row.ChannelID))
			}},
		}
	}
	s.showPopupMenu(items, x, y)
}

func (s *Shell) openServerSettings(guild store.GuildID) {
	s.setIndependentOverlay(NewServerSettings(s.app.Store(), guild, s.styles, func(c store.Channel) { s.openChannelSettings(guild, c) }, func(r store.Role) { s.openRoleOptions(r.ID) }))
}
func (s *Shell) openChannelSettings(guild store.GuildID, c store.Channel) {
	can := s.app.Store().MemberCan(guild, s.app.SelfID(), store.PermManageChannels)
	items := []widget.MenuItem{
		{Label: "Create channel…", Disabled: !can, OnSelect: func() {
			s.closePopup()
			s.setIndependentOverlay(NewPrompt("Create channel", "Channel name…", s.styles, func(name string) { s.app.CreateTextChannel(guild, name) }, s.closeOverlay))
		}},
		{Label: "Rename…", Disabled: !can, OnSelect: func() {
			s.closePopup()
			s.setIndependentOverlay(NewPrompt("Rename channel", "Channel name…", s.styles, func(name string) { s.app.RenameChannel(c.ID, name) }, s.closeOverlay))
		}},
		{Label: "Move up", Disabled: !can || c.Position <= 0, OnSelect: func() { s.closePopup(); s.app.MoveChannel(guild, c.ID, c.Position-1) }},
		{Label: "Move down", Disabled: !can, OnSelect: func() { s.closePopup(); s.app.MoveChannel(guild, c.ID, c.Position+1) }},
		{Separator: true}, {Label: "Delete…", Danger: true, Disabled: !can, OnSelect: func() {
			s.showPopupMenu([]widget.MenuItem{{Label: "Delete — click again", Danger: true, OnSelect: func() { s.closePopup(); s.app.DeleteChannel(c.ID) }}, {Label: "Cancel", OnSelect: s.closePopup}}, 0, 0)
		}},
	}
	s.showPopupMenu(items, 0, 0)
}

// openGuildMenu shows the sidebar context menu for a guild or folder: pin/unpin
// and copy id for guilds, collapse/expand for folders.
func (s *Shell) openGuildMenu(row store.GuildRow, x, y int) {
	var items []widget.MenuItem
	if row.Folder {
		label := "Collapse"
		if row.Collapsed {
			label = "Expand"
		}
		items = []widget.MenuItem{{Label: label, OnSelect: func() {
			s.closePopup()
			s.mv.ToggleCollapseFolder(row.FolderID)
		}}}
	} else {
		pinLabel := "Pin server"
		if s.mv.IsGuildPinned(row.GuildID) {
			pinLabel = "Unpin server"
		}
		items = []widget.MenuItem{
			{Label: "Server settings…", OnSelect: func() { s.closePopup(); s.openServerSettings(row.GuildID) }},
			{Separator: true},
			{Label: pinLabel, OnSelect: func() {
				s.closePopup()
				s.mv.TogglePinGuild(row.GuildID)
			}},
			{Separator: true},
			{Label: "Copy server ID", OnSelect: func() {
				s.closePopup()
				s.copyID("Server ID copied", uint64(row.GuildID))
			}},
		}
	}
	s.showPopupMenu(items, x, y)
}

// openThreadMenu shows the sidebar context menu for a thread sub-item:
// join/leave, archive/unarchive (gated by MANAGE_THREADS or ownership), and copy
// id.
func (s *Shell) openThreadMenu(row store.ChannelRow, x, y int) {
	c, _ := s.app.Store().Channel(row.ChannelID)
	joined := c.Thread != nil && c.Thread.Joined
	archived := c.Thread != nil && c.Thread.Archived
	joinLabel := "Join thread"
	if joined {
		joinLabel = "Leave thread"
	}
	archiveLabel := "Archive thread"
	if archived {
		archiveLabel = "Unarchive thread"
	}
	canManage := s.canManageThread(c)
	pinLabel := "Pin thread"
	if s.mv.IsChannelPinned(row.ChannelID) {
		pinLabel = "Unpin thread"
	}
	items := []widget.MenuItem{
		{Label: joinLabel, OnSelect: func() {
			s.closePopup()
			if joined {
				s.app.LeaveThread(row.ChannelID)
			} else {
				s.app.JoinThread(row.ChannelID)
			}
		}},
		{Label: archiveLabel, Disabled: !canManage, OnSelect: func() {
			s.closePopup()
			s.app.SetThreadArchived(row.ChannelID, !archived)
		}},
		{Separator: true},
		{Label: pinLabel, OnSelect: func() {
			s.closePopup()
			s.mv.TogglePinChannel(row.ChannelID)
		}},
		{Separator: true},
		{Label: "Copy channel ID", OnSelect: func() {
			s.closePopup()
			s.copyID("Channel ID copied", uint64(row.ChannelID))
		}},
	}
	s.showPopupMenu(items, x, y)
}

// openThreadPrompt asks for a name, then creates a message-anchored thread.
func (s *Shell) openThreadPrompt(msg store.Message) {
	s.setIndependentOverlay(NewPrompt("New thread", "Thread name…", s.styles,
		func(name string) {
			s.app.CreateThreadFromMessage(msg.ChannelID, msg.ID, name)
		},
		s.closeOverlay,
	))
}

// canManageThread reports whether the account may archive/unarchive a thread:
// it owns the thread, or holds MANAGE_THREADS in the guild.
func (s *Shell) canManageThread(c store.Channel) bool {
	if c.Thread != nil && c.Thread.OwnerID != 0 && c.Thread.OwnerID == s.app.SelfID() {
		return true
	}
	if c.GuildID == 0 || c.GuildID == app.DirectMessagesGuildID {
		return true
	}
	return s.app.Store().MemberCan(c.GuildID, s.app.SelfID(), store.PermManageThreads)
}

// copyID places a snowflake on the clipboard and reports the outcome.
func (s *Shell) copyID(notice string, id uint64) {
	if err := term.CopyToClipboard(os.Stdout, strconv.FormatUint(id, 10)); err != nil {
		s.ShowToast("Clipboard error", err)
		return
	}
	s.ShowNotice("Copied", notice)
}

// showPopupMenu styles, anchors, and installs a context menu as the active popup.
func (s *Shell) showPopupMenu(items []widget.MenuItem, x, y int) {
	menu := widget.NewMenu(items)
	s.styleMenu(menu)
	menu.SetAnchor(x, y)
	menu.OnDismiss(s.closePopup)
	s.setPopup(menu)
}

func (s *Shell) setPopup(popup tui.Widget) {
	if s == nil {
		return
	}
	if s.cfg.Accessibility.VimNavigation && s.editor.phase != editorNormal {
		s.exitEditor(nil)
	} else {
		s.cancelFocusRequest()
	}
	s.popup = popup
}

func (s *Shell) styleMenu(menu *widget.Menu) {
	if menu == nil {
		return
	}
	menu.SetStyle(s.styles.Cell("menu"))
	menu.SetSelectedStyle(s.styles.Cell("menu.selected"))
	menu.SetBorderStyle(s.styles.Cell("panels.border"))
	menu.SetDangerStyle(s.styles.Cell("menu.danger"))
	menu.SetDisabledStyle(s.styles.Cell("menu.disabled"))
	menu.SetKeyStyle(s.styles.Cell("menu.key"))
}

func (s *Shell) canManageMessages(channel store.ChannelID) bool {
	if s == nil || s.app == nil {
		return false
	}
	if c, ok := s.app.Store().Channel(channel); ok {
		if c.GuildID == app.DirectMessagesGuildID || c.GuildID == 0 {
			return true
		}
		return s.app.Store().MemberCan(c.GuildID, s.app.SelfID(), store.PermManageMessages)
	}
	return false
}

type overlayCloser interface{ Close() }

func (s *Shell) closeOverlay() {
	if s == nil {
		return
	}
	owned := s.overlay != nil && s.composerOverlay != nil && s.overlay == s.composerOverlay
	resumeComposer := owned && s.editor.phase == editorOverlaySuspended && s.editor.channel == s.activeChannel() && s.composerWritable()
	if closer, ok := s.overlay.(overlayCloser); ok {
		closer.Close()
	}
	if s.viewerCancel != nil {
		s.viewerCancel()
		s.viewerCancel = nil
	}
	playingVideo := !s.videoRegion.Empty()
	if playingVideo {
		s.teardownVideo()
	}
	s.overlay = nil
	s.composerOverlay = nil
	if resumeComposer {
		s.beginComposerInput(true)
	} else if s.editor.phase == editorOverlaySuspended {
		s.exitEditor(nil)
	}
	if playingVideo && s.app != nil {
		// mpv painted over the screen outside our frame diff; re-emit everything.
		s.app.ForceRepaint()
	}
}

// Close cancels every Shell-owned background media operation and stops mpv.
// It is safe to call repeatedly and is invoked by main on every run exit.
func (s *Shell) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.lifecycleCancel != nil {
			s.lifecycleCancel()
		}
		if s.viewerCancel != nil {
			s.viewerCancel()
			s.viewerCancel = nil
		}
		s.clipboardMu.Lock()
		clipboardCancel := s.clipboardCancel
		s.clipboardCancel = nil
		s.clipboardBusy = false
		s.clipboardMu.Unlock()
		if clipboardCancel != nil {
			clipboardCancel()
		}
		if s.prefetch != nil {
			s.prefetch.Close()
		}
		// Viewer and clipboard workers observe the canceled lifecycle context and
		// relinquish any unposted temp files before Shell ownership ends.
		s.lifecycleWG.Wait()
		if s.mv != nil {
			// Staged clipboard files are Shell-owned until send; process shutdown
			// must release them just like composer cancellation.
			s.mv.clearAttachments()
			if s.mv.chat != nil {
				s.mv.chat.CloseMedia()
			}
			if s.mv.forumPreview != nil {
				s.mv.forumPreview.CloseMedia()
			}
		}
		if closer, ok := s.overlay.(overlayCloser); ok {
			closer.Close()
		}
		s.exitEditor(nil)
		s.popup = nil
		s.overlay = nil
		s.composerOverlay = nil
		s.teardownVideo()
	})
}

func (s *Shell) closePopup() { s.popup = nil }

// ShowToast displays a dismissible error popup over the active view.
func (s *Shell) ShowToast(title string, err error) {
	if s == nil || err == nil {
		return
	}
	detail := err.Error()
	for i, toast := range s.toasts {
		if toast == nil || toast.title != title || toast.detail != detail || !toast.expiresAt.IsZero() || toast.onActivate != nil {
			continue
		}
		toast.repeats = max(toast.repeats, 1) + 1
		// Bring a repeated older error back to the front without adding another
		// permanent toast to the stack.
		s.toasts = append(s.toasts[:i], s.toasts[i+1:]...)
		s.toasts = append([]*Toast{toast}, s.toasts...)
		return
	}
	toast := NewToast(title, detail, s.styles)
	s.toasts = append([]*Toast{toast}, s.toasts...)
}

// ShowNotice displays a short dismissible informational popup.
func (s *Shell) ShowNotice(title, detail string) {
	if s == nil {
		return
	}
	s.showNotification(title, detail)
}

// ShowTimedNotice shows a notice that auto-dismisses after ttl (unless the user
// expands it). Used for low-importance confirmations like image paste.
func (s *Shell) ShowTimedNotice(title, detail string, ttl time.Duration) {
	if s == nil {
		return
	}
	toast := NewToast(title, detail, s.styles).SetTTL(ttl)
	s.toasts = append([]*Toast{toast}, s.toasts...)
}

func (s *Shell) showNotification(title, detail string) {
	if s == nil {
		return
	}
	toast := newExpiringToast(title, detail, s.styles, s.clock())
	s.toasts = append([]*Toast{toast}, s.toasts...)
}

func (s *Shell) showIncomingMessageToast(message store.Message, title, body string) {
	toast := newExpiringToast(title, body, s.styles, s.clock())
	if message.ChannelID != 0 && s.mv != nil {
		channelID := message.ChannelID
		toast.onActivate = func() {
			// Navigation must not leave a modal or mpv surface covering the newly
			// activated channel. Clear all actionable layers first.
			s.popup = nil
			s.closeOverlay()
			s.mv.NavigateToChannel(channelID)
		}
	}
	s.toasts = append([]*Toast{toast}, s.toasts...)
}

// NotifyIncomingMessage routes an incoming Discord ping to the desktop only
// while this terminal window is unfocused; otherwise it becomes an in-app toast.
func (s *Shell) NotifyIncomingMessage(message store.Message) {
	if s == nil {
		return
	}
	title := strings.TrimSpace(message.Author)
	if title == "" {
		title = "New message"
	}
	body := strings.Join(strings.Fields(message.Content), " ")
	if body == "" {
		body = "Sent an attachment or embed"
	}
	body = tuitext.Truncate(body, 240, tuitext.Ellipsis)
	if s.unfocused && s.notifier != nil {
		s.dispatchNotification(func() {
			if err := s.notifier.Notify(title, body); err != nil {
				s.postNotification(func() { s.showIncomingMessageToast(message, title, body) })
			}
		})
		return
	}
	s.showIncomingMessageToast(message, title, body)
}

// NotifyAccountMessage notifies about a message that arrived on a background
// (non-active) account, prefixing the sender with the account label so the user
// can tell which account received it.
func (s *Shell) NotifyAccountMessage(account string, message store.Message) {
	if s == nil {
		return
	}
	if account != "" {
		message.Author = account + " · " + strings.TrimSpace(message.Author)
	}
	s.NotifyIncomingMessage(message)
}

func (s *Shell) dispatchNotification(fn func()) {
	if s != nil && s.dispatch != nil {
		s.dispatch(fn)
		return
	}
	fn()
}

func (s *Shell) postNotification(fn func()) {
	if s != nil && s.post != nil {
		s.post(fn)
		return
	}
	fn()
}

func (s *Shell) clock() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Shell) expireToasts(now time.Time) bool {
	if s == nil || len(s.toasts) == 0 {
		return false
	}
	kept := make([]*Toast, 0, len(s.toasts))
	for _, toast := range s.toasts {
		if !toast.Expired(now) {
			kept = append(kept, toast)
		}
	}
	changed := len(kept) != len(s.toasts)
	s.toasts = kept
	return changed
}

// Toast returns the newest in-app notification, if any.
func (s *Shell) Toast() *Toast {
	if s == nil || len(s.toasts) == 0 {
		return nil
	}
	return s.toasts[0]
}

// Toasts returns the newest-first notification stack.
func (s *Shell) Toasts() []*Toast { return append([]*Toast(nil), s.toasts...) }
