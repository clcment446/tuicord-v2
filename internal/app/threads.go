package app

import (
	"strconv"
	"strings"
	"time"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)

// archivedPageLimit is how many archived threads to request per "Load
// archived…" page.
const archivedPageLimit = 25

// LoadActiveThreads fetches a guild's active threads once per session, mirroring
// the gated pattern used for history and channels. Threads that already arrived
// via THREAD_LIST_SYNC or GUILD_CREATE mark the guild loaded, so this is a
// guarded fallback for guilds opened without that data.
func (a *App) LoadActiveThreads(guild store.GuildID) {
	if a == nil || a.threads == nil || guild == 0 || guild == DirectMessagesGuildID {
		return
	}
	version, ok := a.beginThreadLoad(guild)
	if !ok {
		return
	}
	generation := a.store.GuildGeneration(guild)
	go a.loadActiveThreadsFrom(guild, generation, version)
}

func (a *App) loadActiveThreads(guild store.GuildID) {
	a.loadActiveThreadsFrom(guild, a.store.GuildGeneration(guild), a.threadLoadVersion(guild))
}

func (a *App) loadActiveThreadsFrom(guild store.GuildID, generation, version uint64) {
	active, err := a.threads.ActiveThreads(discord.GuildID(guild))
	if err != nil {
		a.ui.Post(func() {
			if a.store.GuildGeneration(guild) != generation || !a.threadLoadCurrent(guild, version) {
				return
			}
			a.finishThreadLoad(guild, false)
			a.reportError(err)
		})
		return
	}
	threads := make([]store.Channel, 0, len(active.Threads))
	for _, t := range active.Threads {
		t.GuildID = discord.GuildID(guild)
		threads = append(threads, convertChannel(t))
	}
	joined := make(map[store.ChannelID]bool, len(active.Members))
	for _, m := range active.Members {
		joined[store.ChannelID(m.ID)] = true
	}
	a.ui.Post(func() {
		if a.store.GuildGeneration(guild) != generation || !a.threadLoadCurrent(guild, version) {
			return
		}
		for _, t := range threads {
			if t.Thread != nil && joined[t.ID] {
				t.Thread.Joined = true
			}
			a.store.UpsertChannel(t)
		}
		a.finishThreadLoad(guild, true)
		if a.onChange != nil {
			a.onChange()
		}
	})
}

// LoadArchivedThreads fetches one page of public archived threads for a channel
// (a text/announcement channel or a forum). It is gated per channel so a repeat
// "Load archived…" click while a fetch is in flight is a no-op.
func (a *App) LoadArchivedThreads(channel store.ChannelID) {
	if a == nil || a.threads == nil || channel == 0 {
		return
	}
	version, ok := a.beginArchivedLoad(channel)
	if !ok {
		return
	}
	parent, _ := a.store.Channel(channel)
	generation := a.store.ChannelGeneration(channel)
	before := a.archivedCursor(channel)
	go a.loadArchivedThreadsFrom(channel, parent.GuildID, generation, version, before)
}

func (a *App) loadArchivedThreads(channel store.ChannelID, parentGuild store.GuildID) {
	a.loadArchivedThreadsFrom(channel, parentGuild, a.store.ChannelGeneration(channel), a.archivedLoadVersion(channel), a.archivedCursor(channel))
}

func (a *App) loadArchivedThreadsFrom(channel store.ChannelID, parentGuild store.GuildID, generation, version uint64, before discord.Timestamp) {
	page, err := a.threads.PublicArchivedThreads(discord.ChannelID(channel), before, archivedPageLimit)
	if err != nil {
		a.ui.Post(func() {
			if a.store.ChannelGeneration(channel) != generation || !a.archivedLoadCurrent(channel, version) {
				return
			}
			a.finishArchivedLoad(channel, false, false, discord.Timestamp{})
			a.reportError(err)
		})
		return
	}
	threads := make([]store.Channel, 0, len(page.Threads))
	for _, t := range page.Threads {
		if t.GuildID == 0 {
			t.GuildID = discord.GuildID(parentGuild)
		}
		threads = append(threads, convertChannel(t))
	}
	a.ui.Post(func() {
		if a.store.ChannelGeneration(channel) != generation || !a.archivedLoadCurrent(channel, version) {
			return
		}
		for _, t := range threads {
			a.store.UpsertChannel(t)
		}
		next := archivedCursor(page.Threads)
		a.finishArchivedLoad(channel, true, page.More, next)
		if a.onChange != nil {
			a.onChange()
		}
	})
}

// CreateThreadFromMessage starts a message-anchored public thread on a text or
// announcement channel. The new thread arrives via THREAD_CREATE; failures are
// reported through OnError.
func (a *App) CreateThreadFromMessage(channel store.ChannelID, message store.MessageID, name string) {
	if a == nil || a.threads == nil || channel == 0 || message == 0 || strings.TrimSpace(name) == "" {
		return
	}
	data := api.StartThreadData{Name: name, AutoArchiveDuration: discord.OneDayArchive}
	a.runInBackground(func() error {
		_, err := a.threads.StartThreadWithMessage(discord.ChannelID(channel), discord.MessageID(message), data)
		return err
	})
}

// JoinThread adds the logged-in account to a thread. The membership flip arrives
// via THREAD_MEMBER(S)_UPDATE; on failure OnError fires.
func (a *App) JoinThread(thread store.ChannelID) {
	if a == nil || a.threads == nil || thread == 0 {
		return
	}
	a.runInBackground(func() error {
		return a.threads.JoinThread(discord.ChannelID(thread))
	})
}

// LeaveThread removes the logged-in account from a thread.
func (a *App) LeaveThread(thread store.ChannelID) {
	if a == nil || a.threads == nil || thread == 0 {
		return
	}
	a.runInBackground(func() error {
		return a.threads.LeaveThread(discord.ChannelID(thread))
	})
}

// SetThreadArchived archives or unarchives a thread via a channel edit. The
// store flips optimistically-free once the gateway echoes THREAD_UPDATE; here we
// patch locally on success so the sidebar reflects the change even if the echo
// is delayed.
func (a *App) SetThreadArchived(thread store.ChannelID, archived bool) {
	if a == nil || a.handle == nil || thread == 0 {
		return
	}
	go func() {
		data := api.ModifyChannelData{Archived: option.Bool(&archived)}
		if err := a.handle.ModifyChannel(discord.ChannelID(thread), data); err != nil {
			a.reportError(err)
			return
		}
		a.ui.Post(func() {
			a.store.SetArchived(thread, archived)
			if a.onChange != nil {
				a.onChange()
			}
		})
	}()
}

// Publish crossposts an announcement-channel message to following servers.
func (a *App) Publish(channel store.ChannelID, message store.MessageID) {
	if a == nil || a.send == nil || channel == 0 || message == 0 {
		return
	}
	a.runInBackground(func() error {
		_, err := a.send.CrosspostMessage(discord.ChannelID(channel), discord.MessageID(message))
		return err
	})
}

// forumThreadPayload is the raw body for creating a forum post: a thread name,
// applied tag IDs, and the first message. Snowflakes travel as strings.
type forumThreadPayload struct {
	Name        string             `json:"name"`
	AppliedTags []string           `json:"applied_tags,omitempty"`
	Message     forumThreadMessage `json:"message"`
}

type forumThreadMessage struct {
	Content string `json:"content"`
	Nonce   string `json:"nonce,omitempty"`
}

// CreateForumPost creates a new post (thread with a first message) in a forum
// channel. tagIDs are the forum tag snowflakes to apply. The post arrives via
// THREAD_CREATE; failures are reported through OnError.
func (a *App) CreateForumPost(forum store.ChannelID, title, body string, tagIDs []uint64) {
	if a == nil || a.forum == nil || forum == 0 || strings.TrimSpace(title) == "" {
		return
	}
	payload := forumThreadPayload{
		Name:    title,
		Message: forumThreadMessage{Content: body, Nonce: newNonce()},
	}
	for _, id := range tagIDs {
		payload.AppliedTags = append(payload.AppliedTags, strconv.FormatUint(id, 10))
	}
	a.runInBackground(func() error {
		_, err := a.forum.postForumThread(forum, payload)
		return err
	})
}

// restForumPoster posts a forum-thread create payload through the session's REST
// client, returning the created thread's ID.
type restForumPoster struct {
	sess *session.Session
}

func (r restForumPoster) postForumThread(channel store.ChannelID, p forumThreadPayload) (store.ChannelID, error) {
	url := api.EndpointChannels + strconv.FormatUint(uint64(channel), 10) + "/threads"
	var ch discord.Channel
	if err := r.sess.RequestJSON(&ch, "POST", url, httputil.WithJSONBody(p)); err != nil {
		return 0, err
	}
	return store.ChannelID(ch.ID), nil
}

func (a *App) markThreadsLoaded(guild store.GuildID) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.threadsGate.markLoaded(guild)
}

func (a *App) beginThreadLoad(guild store.GuildID) (uint64, bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.threadsGate.beginVersion(guild)
}

func (a *App) threadLoadVersion(guild store.GuildID) uint64 {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.threadsGate.version[guild]
}

func (a *App) threadLoadCurrent(guild store.GuildID, version uint64) bool {
	return a.threadLoadVersion(guild) == version
}

func (a *App) finishThreadLoad(guild store.GuildID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.threadsGate.finish(guild, ok)
}

func (a *App) beginArchivedLoad(channel store.ChannelID) (uint64, bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.archivedGate.beginVersion(channel)
}

func (a *App) archivedLoadVersion(channel store.ChannelID) uint64 {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.archivedGate.version[channel]
}

func (a *App) archivedLoadCurrent(channel store.ChannelID, version uint64) bool {
	return a.archivedLoadVersion(channel) == version
}

func (a *App) finishArchivedLoad(channel store.ChannelID, ok, more bool, before discord.Timestamp) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	if !more {
		a.archivedGate.finish(channel, ok)
	} else {
		a.archivedGate.finishOlder(channel, false)
	}
	if ok && more {
		a.archivedBefore[channel] = before
	}
}

func (a *App) archivedCursor(channel store.ChannelID) discord.Timestamp {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.archivedBefore[channel]
}

// archivedCursor returns the oldest archive timestamp in a page, which Discord
// accepts as the exclusive `before` cursor for the following page.
func archivedCursor(threads []discord.Channel) discord.Timestamp {
	var oldest discord.Timestamp
	for _, thread := range threads {
		if thread.ThreadMetadata == nil {
			continue
		}
		stamp := thread.ThreadMetadata.ArchiveTimestamp
		if time.Time(oldest).IsZero() || time.Time(stamp).Before(time.Time(oldest)) {
			oldest = stamp
		}
	}
	return oldest
}
