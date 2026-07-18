package plugin

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// writePlugin drops a .lua file into dir and returns dir for use as Options.Dir.
func writePlugin(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name+".lua"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// captureHost records side effects on buffered channels so tests can await the
// asynchronous plugin goroutine without sleeping.
type captureHost struct {
	sent    chan string
	sentTo  chan [2]string
	react   chan [3]string
	notify  chan [2]string
	reply   chan replyCall
	style   chan styleCall
	overlay chan overlayCall
	theme   chan map[string]string
}

type replyCall struct {
	channel uint64
	message uint64
	content string
	mention bool
}

type styleCall struct {
	selector string
	props    map[string]string
}

type overlayCall struct {
	title string
	lines []string
}

func newCaptureHost() (*captureHost, *Host) {
	c := &captureHost{
		sent:    make(chan string, 8),
		sentTo:  make(chan [2]string, 8),
		react:   make(chan [3]string, 8),
		notify:  make(chan [2]string, 8),
		reply:   make(chan replyCall, 8),
		style:   make(chan styleCall, 8),
		overlay: make(chan overlayCall, 8),
		theme:   make(chan map[string]string, 8),
	}
	h := &Host{
		Send:          func(content string) { c.sent <- content },
		SendTo:        func(ch uint64, content string) { c.sentTo <- [2]string{u(ch), content} },
		React:         func(ch, msg uint64, emoji string) { c.react <- [3]string{u(ch), u(msg), emoji} },
		Notify:        func(title, body string) { c.notify <- [2]string{title, body} },
		Reply:         func(ch, msg uint64, content string, mention bool) { c.reply <- replyCall{ch, msg, content, mention} },
		Style:         func(selector string, props map[string]string) { c.style <- styleCall{selector, props} },
		OpenOverlay:   func(title string, lines []string) { c.overlay <- overlayCall{title, lines} },
		ApplyTheme:    func(palette map[string]string) { c.theme <- palette },
		ActiveChannel: func() uint64 { return 111 },
		ActiveGuild:   func() uint64 { return 222 },
		SelfID:        func() uint64 { return 333 },
	}
	return c, h
}

func u(v uint64) string {
	if v == 0 {
		return "0"
	}
	s := ""
	for v > 0 {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	return s
}

func recvStr(t *testing.T, ch chan string) string {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for plugin side effect")
		return ""
	}
}

func recv2(t *testing.T, ch chan [2]string) [2]string {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for plugin side effect")
		return [2]string{}
	}
}

func TestEventDispatchAndSend(t *testing.T) {
	dir := writePlugin(t, "echo", `
		tuicord.on("message.create", function(m)
			tuicord.send("saw: " .. m.content .. " in " .. m.channel_id)
		end)
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}

	m.Emit(EventMessageCreate, map[string]any{
		"content":    "hi",
		"channel_id": uint64(9007199254740993), // > 2^53, must survive as string
	})

	got := recvStr(t, c.sent)
	if got != "saw: hi in 9007199254740993" {
		t.Fatalf("send got %q", got)
	}
}

func TestRunCommand(t *testing.T) {
	dir := writePlugin(t, "greet", `
		tuicord.command("hello", function(args)
			tuicord.notify("hi", args[1] or "world")
		end, "greet someone")
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}

	if !m.RunCommand("hello", []string{"there"}) {
		t.Fatal("RunCommand returned false for a registered command")
	}
	if got := recv2(t, c.notify); got != [2]string{"hi", "there"} {
		t.Fatalf("notify got %v", got)
	}
	if m.RunCommand("nope", nil) {
		t.Fatal("RunCommand returned true for an unregistered command")
	}
	if names := m.CommandNames(); len(names) != 1 || names[0] != "hello" {
		t.Fatalf("CommandNames = %v", names)
	}
}

func TestRunCommandAndKeyRejectSaturatedQueue(t *testing.T) {
	dir := writePlugin(t, "dispatch", `
		tuicord.command("queued", function() error("command unexpectedly ran") end)
		tuicord.keymap("ctrl+j", function() error("key unexpectedly ran") end)
	`)
	var log bytes.Buffer
	m := NewManager(Options{Dir: dir, QueueSize: 1, Log: &log})
	if errs := m.Load(); len(errs) != 0 {
		m.Close()
		t.Fatalf("load errors: %v", errs)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		close(release)
		m.Close()
	}()
	if !m.rt.submit(func() {
		close(started)
		<-release
	}) {
		t.Fatal("failed to submit blocking job")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking job did not start")
	}
	if !m.rt.submit(func() {}) {
		t.Fatal("failed to fill plugin queue")
	}

	if m.RunCommand("queued", nil) {
		t.Fatal("RunCommand reported handled when the queue was saturated")
	}
	if m.RunKey("ctrl+j") {
		t.Fatal("RunKey reported handled when the queue was saturated")
	}
	if got := log.String(); !strings.Contains(got, `dropped command "queued"`) || !strings.Contains(got, `dropped key "ctrl+j"`) {
		t.Fatalf("dispatch rejection was not logged: %q", got)
	}
}

func TestRunCommandAndKeyRejectAfterShutdown(t *testing.T) {
	dir := writePlugin(t, "dispatch", `
		tuicord.command("stopped", function() error("command unexpectedly ran") end)
		tuicord.keymap("ctrl+k", function() error("key unexpectedly ran") end)
	`)
	var log bytes.Buffer
	m := NewManager(Options{Dir: dir, Log: &log})
	if errs := m.Load(); len(errs) != 0 {
		m.Close()
		t.Fatalf("load errors: %v", errs)
	}
	m.Close()

	if m.RunCommand("stopped", nil) {
		t.Fatal("RunCommand reported handled after shutdown")
	}
	if m.RunKey("ctrl+k") {
		t.Fatal("RunKey reported handled after shutdown")
	}
	if got := log.String(); !strings.Contains(got, `dropped command "stopped"`) || !strings.Contains(got, `dropped key "ctrl+k"`) {
		t.Fatalf("shutdown dispatch rejection was not logged: %q", got)
	}
}

func TestStateAccessorsAsStrings(t *testing.T) {
	dir := writePlugin(t, "ids", `
		tuicord.on("ready", function()
			tuicord.notify(tuicord.active_channel(), tuicord.self_id())
		end)
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	m.Load()

	m.Emit(EventReady, nil)
	if got := recv2(t, c.notify); got != [2]string{"111", "333"} {
		t.Fatalf("accessor notify got %v", got)
	}
}

func TestSandboxBlocksOSAndIO(t *testing.T) {
	dir := writePlugin(t, "evil", `os.execute("touch /tmp/should-not-happen")`)
	_, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	errs := m.Load()
	if len(errs) == 0 {
		t.Fatal("expected a load error for a plugin using os in the sandbox")
	}
}

func TestDisabledPluginSkipped(t *testing.T) {
	dir := writePlugin(t, "skipme", `tuicord.command("x", function() end)`)
	_, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h, Disabled: []string{"skipme"}})
	defer m.Close()
	m.Load()
	if len(m.Loaded()) != 0 {
		t.Fatalf("disabled plugin was loaded: %v", m.Loaded())
	}
}

func TestCallbackErrorIsIsolated(t *testing.T) {
	dir := writePlugin(t, "boom", `
		tuicord.on("message.create", function() error("kaboom") end)
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	m.Load()

	// A callback that errors must surface via Notify (onCallbackError) and not
	// panic the plugin goroutine.
	m.Emit(EventMessageCreate, map[string]any{"content": "x"})
	got := recv2(t, c.notify)
	if got[0] != "Plugin error: boom" {
		t.Fatalf("expected plugin error notify, got %v", got)
	}
}

func TestFSGrantConfinedToDataDir(t *testing.T) {
	dir := writePlugin(t, "store", `
		tuicord.on("ready", function()
			assert(tuicord.fs.write("note.txt", "hello"))
			local data = tuicord.fs.read("note.txt")
			tuicord.notify("read", data)
			-- escaping the data dir must fail
			local ok = tuicord.fs.write("../escape.txt", "no")
			if ok then tuicord.notify("escaped", "!") end
		end)
	`)
	dataDir := t.TempDir()
	c, h := newCaptureHost()
	m := NewManager(Options{
		Dir:     dir,
		DataDir: dataDir,
		Host:    h,
		Grants:  map[string][]string{"store": {CapFS}},
	})
	defer m.Close()
	m.Load()

	m.Emit(EventReady, nil)
	if got := recv2(t, c.notify); got != [2]string{"read", "hello"} {
		t.Fatalf("fs read got %v", got)
	}
	// The confined file exists under the plugin's own data dir.
	if _, err := os.Stat(filepath.Join(dataDir, "store", "note.txt")); err != nil {
		t.Fatalf("confined file missing: %v", err)
	}
	// The escape attempt must not have created a file above the data dir.
	if _, err := os.Stat(filepath.Join(dataDir, "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("fs grant allowed escaping the data dir")
	}
}

func TestFSAbsentWithoutGrant(t *testing.T) {
	dir := writePlugin(t, "nogrant", `
		tuicord.on("ready", function()
			if tuicord.fs == nil then tuicord.notify("fs", "absent") end
		end)
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	m.Load()

	m.Emit(EventReady, nil)
	if got := recv2(t, c.notify); got != [2]string{"fs", "absent"} {
		t.Fatalf("expected fs absent, got %v", got)
	}
}

func TestFSEmptyDataDirDisablesGrantedAPI(t *testing.T) {
	dir := writePlugin(t, "empty", `
		tuicord.on("ready", function()
			if tuicord.fs == nil then
				tuicord.notify("fs", "disabled")
			else
				tuicord.notify("fs", "exposed")
			end
		end)
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{
		Dir:    dir,
		Host:   h,
		Grants: map[string][]string{"empty": {CapFS}},
		// DataDir is intentionally empty: a grant must not turn it into / or .
	})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}
	m.Emit(EventReady, nil)
	if got := recv2(t, c.notify); got != [2]string{"fs", "disabled"} {
		t.Fatalf("empty DataDir exposed filesystem API: %v", got)
	}
}

func TestFSRejectsAbsoluteTraversalAndSymlinkEscapes(t *testing.T) {
	dataDir := t.TempDir()
	pluginDir := filepath.Join(dataDir, "store")
	outside := t.TempDir()
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(pluginDir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	body := fmt.Sprintf(`
		tuicord.on("ready", function()
			assert(not tuicord.fs.write("../traversal.txt", "bad"))
			assert(not tuicord.fs.write(%q, "bad"))
			assert(not tuicord.fs.write("link/symlink.txt", "bad"))
			local value = tuicord.fs.read("link/secret.txt")
			assert(value == nil)
			tuicord.notify("fs", "confined")
		end)
	`, filepath.Join(outside, "absolute.txt"))
	dir := writePlugin(t, "store", body)
	c, h := newCaptureHost()
	m := NewManager(Options{
		Dir:     dir,
		DataDir: dataDir,
		Host:    h,
		Grants:  map[string][]string{"store": {CapFS}},
	})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}
	m.Emit(EventReady, nil)
	if got := recv2(t, c.notify); got != [2]string{"fs", "confined"} {
		t.Fatalf("filesystem escape check did not finish: %v", got)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "traversal.txt")); !os.IsNotExist(err) {
		t.Fatalf("parent traversal created a file: %v", err)
	}
	for _, name := range []string{"absolute.txt", "symlink.txt"} {
		if _, err := os.Stat(filepath.Join(outside, name)); !os.IsNotExist(err) {
			t.Fatalf("filesystem escape created %s: %v", name, err)
		}
	}
}

func TestExamplePluginsLoad(t *testing.T) {
	// The shipped samples must stay valid against the live API.
	_, h := newCaptureHost()
	m := NewManager(Options{Dir: filepath.Join("..", "..", "examples", "plugins"), Host: h})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("example plugins failed to load: %v", errs)
	}
	loaded := m.Loaded()
	if !contains(loaded, "hello") || !contains(loaded, "everything") {
		t.Fatalf("expected both example plugins loaded, got %v", loaded)
	}
	for _, want := range []string{"hi", "about", "status", "echo", "paint", "fscheck"} {
		if !contains(m.CommandNames(), want) {
			t.Fatalf("example command %q not registered; have %v", want, m.CommandNames())
		}
	}
	for _, want := range []string{"dracula", "nord"} {
		if !contains(m.ThemeNames(), want) {
			t.Fatalf("example theme %q not registered; have %v", want, m.ThemeNames())
		}
	}
}

// TestEverythingPluginExercisesAllSurfaces loads the shipped everything.lua in
// isolation (with the fs grant) and drives every command, asserting each API
// surface reaches the host. It is the closest automated stand-in for a manual
// run of the plugin system.
func TestEverythingPluginExercisesAllSurfaces(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "plugins", "everything.lua"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "everything.lua"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()

	c, h := newCaptureHost()
	m := NewManager(Options{
		Dir:     dir,
		DataDir: dataDir,
		Host:    h,
		Grants:  map[string][]string{"everything": {CapFS}},
	})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("everything.lua failed to load: %v", errs)
	}

	const chanID = uint64(555)
	const msgID = uint64(9007199254740993) // > 2^53, must survive as a string

	// A message.create event records the last message for reply/react below.
	m.Emit(EventMessageCreate, map[string]any{
		"channel_id": chanID, "id": msgID, "content": "hi", "bot": false,
	})

	// ;echo -> tuicord.send
	m.RunCommand("echo", []string{"hello", "world"})
	if got := recvStr(t, c.sent); got != "hello world" {
		t.Fatalf("echo/send = %q", got)
	}

	// ;dm -> tuicord.send_to
	m.RunCommand("dm", []string{"555", "yo", "there"})
	select {
	case got := <-c.sentTo:
		if got != [2]string{"555", "yo there"} {
			t.Fatalf("dm/send_to = %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on send_to")
	}

	// ;quote -> tuicord.reply against the recorded last message (string-safe id)
	m.RunCommand("quote", []string{"nice"})
	select {
	case got := <-c.reply:
		if got.channel != chanID || got.message != msgID || got.content != "nice" || got.mention {
			t.Fatalf("quote/reply = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on reply")
	}

	// ;thumbsup -> tuicord.react with the 👍 emoji
	m.RunCommand("thumbsup", nil)
	select {
	case got := <-c.react:
		if got[0] != "555" || got[1] != "9007199254740993" || got[2] != "\U0001F44D" {
			t.Fatalf("thumbsup/react = %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on react")
	}

	// ;paint -> tuicord.style
	m.RunCommand("paint", nil)
	select {
	case got := <-c.style:
		if got.selector != "messages.author" || got.props["fg"] != "#ff0000" || got.props["bold"] != "true" {
			t.Fatalf("paint/style = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on style")
	}
	if got := recv2(t, c.notify); got[0] != "theme" {
		t.Fatalf("paint notify = %v", got)
	}

	// ;status -> tuicord.overlay, populated with the live accessors
	m.RunCommand("status", nil)
	select {
	case got := <-c.overlay:
		if got.title != "Plugin self-test" {
			t.Fatalf("status overlay title = %q", got.title)
		}
		if !anyContains(got.lines, "active channel: 111") || !anyContains(got.lines, "self id:        333") {
			t.Fatalf("status overlay missing accessor lines: %v", got.lines)
		}
		if !anyContains(got.lines, "message.create: 1") {
			t.Fatalf("status overlay missing event count: %v", got.lines)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on overlay")
	}

	// ;fscheck -> fs grant round-trip, reports the value it read back
	m.RunCommand("fscheck", nil)
	if got := recv2(t, c.notify); got[0] != "fs" || got[1] != "wrote and read back: 1" {
		t.Fatalf("fscheck notify = %v", got)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "everything", "counter.txt")); err != nil {
		t.Fatalf("fs grant did not write into the data dir: %v", err)
	}

	// ;netcheck -> net not granted here, so it reports that (no network hit)
	m.RunCommand("netcheck", nil)
	if got := recv2(t, c.notify); got[0] != "net" || !anyContains([]string{got[1]}, "not granted") {
		t.Fatalf("netcheck notify = %v", got)
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func anyContains(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func TestStyleBinding(t *testing.T) {
	dir := writePlugin(t, "theme", `
		tuicord.on("ready", function()
			tuicord.style("messages.author", { fg = "#ff0000", bold = true })
		end)
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	m.Load()

	m.Emit(EventReady, nil)
	select {
	case got := <-c.style:
		if got.selector != "messages.author" || got.props["fg"] != "#ff0000" || got.props["bold"] != "true" {
			t.Fatalf("style call = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for style call")
	}
}

func TestOverlayBinding(t *testing.T) {
	dir := writePlugin(t, "panel", `
		tuicord.command("show", function()
			tuicord.overlay("My Panel", {"line one", "line two"})
		end)
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	m.Load()

	m.RunCommand("show", nil)
	select {
	case got := <-c.overlay:
		if got.title != "My Panel" || len(got.lines) != 2 || got.lines[0] != "line one" {
			t.Fatalf("overlay call = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for overlay call")
	}
}

func TestThemeRegisterAndApply(t *testing.T) {
	dir := writePlugin(t, "palettes", `
		tuicord.theme("dracula", { background = "#282a36", accent = "#bd93f9" })
		tuicord.theme("nord", { background = "#2e3440", accent = "#88c0d0" })
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}

	if got, want := m.ThemeNames(), []string{"dracula", "nord"}; !equalSlice(got, want) {
		t.Fatalf("ThemeNames = %v, want %v", got, want)
	}
	if m.ApplyTheme("does-not-exist") {
		t.Fatal("ApplyTheme returned true for an unknown theme")
	}
	if !m.ApplyTheme("dracula") {
		t.Fatal("ApplyTheme returned false for a registered theme")
	}
	select {
	case palette := <-c.theme:
		if palette["background"] != "#282a36" || palette["accent"] != "#bd93f9" {
			t.Fatalf("applied palette = %v", palette)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ApplyTheme")
	}
}

func TestLoadConfigLua(t *testing.T) {
	// config.lua is loaded via LoadConfig regardless of the plugins dir, and can
	// register keybindings and commands like any Lua context.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.lua")
	body := `
		tuicord.keymap("ctrl+j", function() tuicord.send("from config") end)
		tuicord.command("cfg", function() tuicord.notify("cfg", "ran") end)
	`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, h := newCaptureHost()
	// Point Dir at an empty dir: only config.lua should load.
	m := NewManager(Options{Dir: t.TempDir(), Host: h})
	defer m.Close()
	if err := m.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if specs := m.KeySpecs(); len(specs) != 1 || specs[0] != "ctrl+j" {
		t.Fatalf("config keymap not registered: %v", specs)
	}
	if !m.RunKey("ctrl+j") {
		t.Fatal("RunKey returned false for config-bound key")
	}
	if got := recvStr(t, c.sent); got != "from config" {
		t.Fatalf("config keymap send = %q", got)
	}
	if !contains(m.Loaded(), "config") {
		t.Fatalf("config context not marked loaded: %v", m.Loaded())
	}
}

func TestLoadConfigMissingIsNoError(t *testing.T) {
	_, h := newCaptureHost()
	m := NewManager(Options{Dir: t.TempDir(), Host: h})
	defer m.Close()
	if err := m.LoadConfig(filepath.Join(t.TempDir(), "config.lua")); err != nil {
		t.Fatalf("missing config.lua should be no error: %v", err)
	}
}

func TestMissingDirIsNoError(t *testing.T) {
	_, h := newCaptureHost()
	m := NewManager(Options{Dir: filepath.Join(t.TempDir(), "does-not-exist"), Host: h})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("missing dir should not error: %v", errs)
	}
}

func TestFailedStartupRestoresShadowedRegistrations(t *testing.T) {
	dir := t.TempDir()
	good := `
		tuicord.command("shared", function() tuicord.notify("command", "original") end, "original help")
		tuicord.keymap("ctrl+r", function() tuicord.notify("key", "original") end)
		tuicord.theme("shared", { background = "#111111" })
	`
	failed := `
		tuicord.command("shared", function() tuicord.notify("command", "failed") end, "failed help")
		tuicord.keymap("ctrl+r", function() tuicord.notify("key", "failed") end)
		tuicord.theme("shared", { background = "#ffffff" })
		tuicord.theme("leaked", { background = "#badbad" })
		error("startup failed")
	`
	if err := os.WriteFile(filepath.Join(dir, "a-original.lua"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b-failed.lua"), []byte(failed), 0o644); err != nil {
		t.Fatal(err)
	}

	c, h := newCaptureHost()
	m := NewManager(Options{Dir: dir, Host: h})
	defer m.Close()
	if errs := m.Load(); len(errs) != 1 {
		t.Fatalf("load errors = %v, want one failed plugin", errs)
	}

	if got, want := m.CommandNames(), []string{"shared"}; !equalSlice(got, want) {
		t.Fatalf("commands after rollback = %v, want %v", got, want)
	}
	if specs := m.KeySpecs(); len(specs) != 1 || specs[0] != "ctrl+r" {
		t.Fatalf("keys after rollback = %v", specs)
	}
	if got, want := m.ThemeNames(), []string{"shared"}; !equalSlice(got, want) {
		t.Fatalf("themes after rollback = %v, want %v", got, want)
	}

	if !m.RunCommand("shared", nil) {
		t.Fatal("restored command is not registered")
	}
	if got := recv2(t, c.notify); got != [2]string{"command", "original"} {
		t.Fatalf("restored command invoked %v", got)
	}
	if !m.RunKey("ctrl+r") {
		t.Fatal("restored key is not registered")
	}
	if got := recv2(t, c.notify); got != [2]string{"key", "original"} {
		t.Fatalf("restored key invoked %v", got)
	}
	if !m.ApplyTheme("shared") {
		t.Fatal("restored theme is not registered")
	}
	select {
	case palette := <-c.theme:
		if palette["background"] != "#111111" {
			t.Fatalf("restored theme palette = %v", palette)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for restored theme")
	}
}

func TestHTTPGetUsesLuaCallbackDeadline(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	dir := writePlugin(t, "net", fmt.Sprintf(`
		tuicord.on("request", function()
			tuicord.http.get(%q)
		end)
	`, server.URL))
	c, h := newCaptureHost()
	m := NewManager(Options{
		Dir:             dir,
		Host:            h,
		Grants:          map[string][]string{"net": {CapNet}},
		CallbackTimeout: 50 * time.Millisecond,
	})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}

	began := time.Now()
	m.Emit("request", nil)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("HTTP request did not start")
	}
	if got := recv2(t, c.notify); got[0] != "Plugin error: net" {
		t.Fatalf("deadline error notification = %v", got)
	}
	if elapsed := time.Since(began); elapsed > time.Second {
		t.Fatalf("Lua deadline took %v to interrupt HTTP request", elapsed)
	}
}

func TestCloseCancelsHTTPGet(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()

	dir := writePlugin(t, "net", fmt.Sprintf(`
		tuicord.on("request", function()
			tuicord.http.get(%q)
		end)
	`, server.URL))
	_, h := newCaptureHost()
	m := NewManager(Options{
		Dir:             dir,
		Host:            h,
		Grants:          map[string][]string{"net": {CapNet}},
		CallbackTimeout: time.Minute,
	})
	if errs := m.Load(); len(errs) != 0 {
		close(release)
		m.Close()
		t.Fatalf("load errors: %v", errs)
	}
	m.Emit("request", nil)
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		m.Close()
		t.Fatal("HTTP request did not start")
	}

	done := make(chan struct{})
	go func() {
		m.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("Manager.Close did not interrupt HTTP request")
	}
	close(release)
}

func TestInfiniteStartupIsBounded(t *testing.T) {
	dir := writePlugin(t, "startup", `
		tuicord.command("stale", function() end)
		tuicord.on("stale", function() end)
		while true do end
	`)
	m := NewManager(Options{
		Dir:            dir,
		StartupTimeout: 50 * time.Millisecond,
	})
	defer m.Close()
	started := time.Now()
	errs := m.Load()
	if len(errs) != 1 {
		t.Fatalf("infinite startup errors = %v, want one deadline error", errs)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("infinite startup took %v", elapsed)
	}
	if m.RunCommand("stale", nil) {
		t.Fatal("failed startup left a callback to a closed state registered")
	}
	m.Emit("stale", nil)
}

func TestInfiniteCallbackTimesOutAndWorkerContinues(t *testing.T) {
	dir := writePlugin(t, "loop", `
		tuicord.on("loop", function()
			tuicord.notify("loop", "started")
			while true do end
		end)
		tuicord.command("after", function()
			tuicord.notify("after", "ran")
		end)
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{
		Dir:             dir,
		Host:            h,
		CallbackTimeout: 50 * time.Millisecond,
	})
	defer m.Close()
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}
	m.Emit("loop", nil)
	if got := recv2(t, c.notify); got != [2]string{"loop", "started"} {
		t.Fatalf("callback did not start: %v", got)
	}
	if got := recv2(t, c.notify); got[0] != "Plugin error: loop" {
		t.Fatalf("infinite callback did not report timeout: %v", got)
	}
	if !m.RunCommand("after", nil) {
		t.Fatal("post-timeout command was not registered")
	}
	if got := recv2(t, c.notify); got != [2]string{"after", "ran"} {
		t.Fatalf("worker did not continue after callback timeout: %v", got)
	}
}

func TestCloseCancelsInfiniteCallbackAndIsBounded(t *testing.T) {
	dir := writePlugin(t, "loop", `
		tuicord.on("loop", function()
			tuicord.notify("loop", "started")
			while true do end
		end)
	`)
	c, h := newCaptureHost()
	m := NewManager(Options{
		Dir:             dir,
		Host:            h,
		CallbackTimeout: time.Minute,
	})
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}
	m.Emit("loop", nil)
	if got := recv2(t, c.notify); got != [2]string{"loop", "started"} {
		t.Fatalf("callback did not start: %v", got)
	}
	started := time.Now()
	m.Close()
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Close waited %v for infinite callback", elapsed)
	}
}

func TestConcurrentEmitAndClose(t *testing.T) {
	dir := writePlugin(t, "events", `tuicord.on("event", function() end)`)
	var log bytes.Buffer
	m := NewManager(Options{Dir: dir, QueueSize: 1, Log: &log})
	if errs := m.Load(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 500 {
				m.Emit("event", nil)
			}
		}()
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		m.Close()
	}()
	go func() {
		defer wg.Done()
		<-start
		m.Close()
	}()
	close(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent Emit/Close did not finish")
	}
}
