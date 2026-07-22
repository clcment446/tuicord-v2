// Package uistate persists per-user, per-client machine state: local view
// preferences, the token-key account registry, and preferred interactive auth
// mode. Tokens themselves remain in the OS keyring.
//
// It is deliberately separate from package config. config.lua is hand-editable
// user configuration; this file is machine-managed churn that the client writes
// as the user clicks around, so it lives under the XDG state directory
// (~/.local/state/tuicord/ui.toml) rather than the config directory.
//
//	st, _ := uistate.Load()          // empty state when the file is absent
//	if st.TogglePinnedChannel(id) {  // returns the new pinned status
//		_ = st.Save()
//	}
package uistate

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"awesomeProject/internal/atomicfile"
	"github.com/BurntSushi/toml"
)

// AppName is the state-directory namespace.
const AppName = "tuicord"

// Account is one machine-managed account registry entry. Tokens remain in the
// OS keyring; this state stores only the stable key, learned label, and user ID.
type Account struct {
	Key   string `toml:"key"`
	Label string `toml:"label"`
	ID    uint64 `toml:"id"`
}

// Accounts is the machine-managed multi-account registry.
type Accounts struct {
	Active int       `toml:"active"`
	List   []Account `toml:"list"`
}

// State is the persisted set of client-side view preferences and small pieces
// of machine-managed startup churn. Guild/channel/user IDs are Discord
// snowflakes. The zero value is a valid, empty state.
type State struct {
	PinnedGuilds   []uint64 `toml:"pinned_guilds"`
	PinnedChannels []uint64 `toml:"pinned_channels"`
	// CollapsedFolders is retained only to decode pre-layout state. loadFrom
	// migrates it into account-scoped GuildLayouts and clears it before save.
	CollapsedFolders    []int64       `toml:"collapsed_folders,omitempty"`
	CollapsedCategories []uint64      `toml:"collapsed_categories"`
	GuildLayouts        []GuildLayout `toml:"guild_layouts"`
	RecentStickers      []uint64      `toml:"recent_stickers"`
	FavoriteEmojis      []string      `toml:"favorite_emojis"`
	FavoriteStickers    []uint64      `toml:"favorite_stickers"`
	Accounts            *Accounts     `toml:"accounts"`
	AuthPreferredMode   string        `toml:"auth_preferred_mode"`
}

type GuildGroup struct {
	ID       int64    `toml:"id"`
	Name     string   `toml:"name"`
	Color    uint32   `toml:"color"`
	GuildIDs []uint64 `toml:"guild_ids"`
}

type GuildLayout struct {
	// AccountKey is the stable registry identity. AccountID remains so layouts
	// written by older releases can be found and associated with their key.
	AccountKey       string       `toml:"account_key,omitempty"`
	AccountID        uint64       `toml:"account_id"`
	CollapsedFolders []int64      `toml:"collapsed_folders,omitempty"`
	Groups           []GuildGroup `toml:"groups"`
}

// AccountList returns a copy of the registry list, or nil when it has not been
// initialized yet.
func (s *State) AccountList() []Account {
	if s == nil || s.Accounts == nil {
		return nil
	}
	return append([]Account(nil), s.Accounts.List...)
}

// ActiveAccount returns the stored active index clamped to the registry.
func (s *State) ActiveAccount() int {
	if s == nil || s.Accounts == nil || len(s.Accounts.List) == 0 || s.Accounts.Active < 0 || s.Accounts.Active >= len(s.Accounts.List) {
		return 0
	}
	return s.Accounts.Active
}

func (s *State) ToggleFavoriteEmoji(key string) bool {
	if s == nil || key == "" {
		return false
	}
	for i, existing := range s.FavoriteEmojis {
		if existing == key {
			s.FavoriteEmojis = append(s.FavoriteEmojis[:i], s.FavoriteEmojis[i+1:]...)
			return false
		}
	}
	s.FavoriteEmojis = append(s.FavoriteEmojis, key)
	return true
}

func (s *State) IsFavoriteEmoji(key string) bool {
	for _, existing := range s.FavoriteEmojis {
		if existing == key {
			return true
		}
	}
	return false
}

func (s *State) ToggleFavoriteSticker(id uint64) bool {
	if s == nil || id == 0 {
		return false
	}
	for i, existing := range s.FavoriteStickers {
		if existing == id {
			s.FavoriteStickers = append(s.FavoriteStickers[:i], s.FavoriteStickers[i+1:]...)
			return false
		}
	}
	s.FavoriteStickers = append(s.FavoriteStickers, id)
	return true
}

func (s *State) IsFavoriteSticker(id uint64) bool {
	for _, existing := range s.FavoriteStickers {
		if existing == id {
			return true
		}
	}
	return false
}

const recentStickerLimit = 20

// RecordRecentSticker moves id to the front of the bounded recent-sticker list.
func (s *State) RecordRecentSticker(id uint64) {
	if s == nil || id == 0 {
		return
	}
	next := make([]uint64, 0, recentStickerLimit)
	next = append(next, id)
	for _, existing := range s.RecentStickers {
		if existing != id {
			next = append(next, existing)
		}
		if len(next) == recentStickerLimit {
			break
		}
	}
	s.RecentStickers = next
}

// Path returns the state-file path, honoring XDG_STATE_HOME and falling back to
// ~/.local/state per the XDG Base Directory spec.
func Path() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, AppName, "ui.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", AppName, "ui.toml"), nil
}

// Load reads the state file. A missing file is not an error: it returns an empty
// State so a fresh install starts clean. Decode errors are returned so a corrupt
// file is surfaced rather than silently discarded.
func Load() (*State, error) {
	path, err := Path()
	if err != nil {
		return &State{}, err
	}
	return loadFrom(path)
}

func loadFrom(path string) (*State, error) {
	st := &State{}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := toml.Unmarshal(data, st); err != nil {
		return st, err
	}
	st.migrateGuildLayouts()
	return st, nil
}

// Save writes the state file, creating the directory when needed. It writes to a
// temporary sibling and renames it into place so a crash mid-write cannot leave
// a truncated file.
func (s *State) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	return s.saveTo(path)
}

func (s *State) saveTo(path string) error {
	// Accounts may have been seeded from legacy config after Load returned.
	// Re-running the idempotent migration here associates any anonymous layout
	// before it is written back.
	s.migrateGuildLayouts()
	return atomicfile.Write(path, 0o600, func(w io.Writer) error {
		return toml.NewEncoder(w).Encode(s)
	})
}

// TogglePinnedGuild flips a guild's pinned status and returns the new value.
func (s *State) TogglePinnedGuild(id uint64) bool { return toggleU64(&s.PinnedGuilds, id) }

// TogglePinnedChannel flips a channel's pinned status and returns the new value.
func (s *State) TogglePinnedChannel(id uint64) bool { return toggleU64(&s.PinnedChannels, id) }

// ToggleCollapsedCategory flips a category's collapsed status and returns the new value.
func (s *State) ToggleCollapsedCategory(id uint64) bool {
	return toggleU64(&s.CollapsedCategories, id)
}

// IsPinnedGuild reports whether a guild is pinned.
func (s *State) IsPinnedGuild(id uint64) bool { return hasU64(s.PinnedGuilds, id) }

// IsPinnedChannel reports whether a channel is pinned.
func (s *State) IsPinnedChannel(id uint64) bool { return hasU64(s.PinnedChannels, id) }

// IsCategoryCollapsed reports whether a category is collapsed.
func (s *State) IsCategoryCollapsed(id uint64) bool { return hasU64(s.CollapsedCategories, id) }

// ToggleCollapsedFolder flips a folder's collapsed status for one account and
// returns the new value.
func (s *State) ToggleCollapsedFolder(accountKey string, accountID uint64, id int64) bool {
	layout := s.ensureGuildLayout(accountKey, accountID)
	if layout == nil {
		return false
	}
	return toggleI64(&layout.CollapsedFolders, id)
}

// IsFolderCollapsed reports whether a folder is collapsed for one account.
func (s *State) IsFolderCollapsed(accountKey string, accountID uint64, id int64) bool {
	layout := s.findGuildLayout(accountKey, accountID)
	return layout != nil && hasI64(layout.CollapsedFolders, id)
}

// CollapsedFolderSet returns one account's collapsed folders as a set for
// store.OrderGuilds.
func (s *State) CollapsedFolderSet(accountKey string, accountID uint64) map[int64]bool {
	layout := s.findGuildLayout(accountKey, accountID)
	if layout == nil {
		return map[int64]bool{}
	}
	out := make(map[int64]bool, len(layout.CollapsedFolders))
	for _, id := range layout.CollapsedFolders {
		out[id] = true
	}
	return out
}

// GuildLayout returns a defensive copy of one account's custom guild groups.
// A collapse-only layout has nil Groups and therefore does not override the
// server-provided folder ordering.
func (s *State) GuildLayout(accountKey string, accountID uint64) ([]GuildGroup, bool) {
	layout := s.findGuildLayout(accountKey, accountID)
	if layout == nil || layout.Groups == nil {
		return nil, false
	}
	return copyGroups(layout.Groups), true
}

func (s *State) SetGuildLayout(accountKey string, accountID uint64, groups []GuildGroup) {
	layout := s.ensureGuildLayout(accountKey, accountID)
	if layout != nil {
		layout.Groups = copyGroups(groups)
	}
}

// findGuildLayout gives the stable key precedence, then adopts an old ID-only
// layout. AccountID zero is never used as an identity, which keeps multiple
// accounts saved before READY distinct.
func (s *State) findGuildLayout(accountKey string, accountID uint64) *GuildLayout {
	if s == nil {
		return nil
	}
	if accountKey != "" {
		for i := range s.GuildLayouts {
			if s.GuildLayouts[i].AccountKey == accountKey {
				if accountID != 0 {
					s.GuildLayouts[i].AccountID = accountID
				}
				return &s.GuildLayouts[i]
			}
		}
	}
	if accountID != 0 {
		for i := range s.GuildLayouts {
			layout := &s.GuildLayouts[i]
			if layout.AccountKey == "" && layout.AccountID == accountID {
				layout.AccountKey = accountKey
				return layout
			}
		}
	}
	// Old layouts written before READY had neither a usable ID nor a key. A
	// single anonymous layout can only belong to the account now opening it.
	// Refuse to guess if malformed state contains more than one candidate.
	anonymous := -1
	for i := range s.GuildLayouts {
		if s.GuildLayouts[i].AccountKey == "" && s.GuildLayouts[i].AccountID == 0 {
			if anonymous >= 0 {
				return nil
			}
			anonymous = i
		}
	}
	if anonymous >= 0 && accountKey != "" {
		s.GuildLayouts[anonymous].AccountKey = accountKey
		s.GuildLayouts[anonymous].AccountID = accountID
		return &s.GuildLayouts[anonymous]
	}
	return nil
}

func (s *State) ensureGuildLayout(accountKey string, accountID uint64) *GuildLayout {
	if s == nil {
		return nil
	}
	if layout := s.findGuildLayout(accountKey, accountID); layout != nil {
		return layout
	}
	s.GuildLayouts = append(s.GuildLayouts, GuildLayout{AccountKey: accountKey, AccountID: accountID})
	return &s.GuildLayouts[len(s.GuildLayouts)-1]
}

// migrateGuildLayouts associates old ID-keyed layouts with registry keys and
// moves the former global collapse list into every account it previously
// affected. An anonymous AccountID-zero layout is assigned to the active
// registry account, which is the only sensible owner older state can identify.
func (s *State) migrateGuildLayouts() {
	if s == nil {
		return
	}
	accounts := s.AccountList()
	for i := range s.GuildLayouts {
		layout := &s.GuildLayouts[i]
		if layout.AccountKey != "" {
			continue
		}
		if layout.AccountID != 0 {
			for _, account := range accounts {
				if account.ID == layout.AccountID && account.Key != "" {
					layout.AccountKey = account.Key
					break
				}
			}
			continue
		}
		if len(accounts) > 0 {
			active := accounts[s.ActiveAccount()]
			layout.AccountKey = active.Key
			layout.AccountID = active.ID
		}
	}

	if len(s.CollapsedFolders) == 0 {
		return
	}
	legacy := uniqueI64(s.CollapsedFolders)
	for i := range s.GuildLayouts {
		s.GuildLayouts[i].CollapsedFolders = mergeI64(s.GuildLayouts[i].CollapsedFolders, legacy)
	}
	for _, account := range accounts {
		layout := s.ensureGuildLayout(account.Key, account.ID)
		layout.CollapsedFolders = mergeI64(layout.CollapsedFolders, legacy)
	}
	if len(s.GuildLayouts) == 0 {
		s.GuildLayouts = append(s.GuildLayouts, GuildLayout{CollapsedFolders: append([]int64(nil), legacy...)})
	}
	s.CollapsedFolders = nil
}

func uniqueI64(list []int64) []int64 {
	return mergeI64(nil, list)
}

func mergeI64(dst, src []int64) []int64 {
	out := append([]int64(nil), dst...)
	for _, id := range src {
		if !hasI64(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func copyGroups(groups []GuildGroup) []GuildGroup {
	out := make([]GuildGroup, len(groups))
	for i, group := range groups {
		out[i] = group
		out[i].GuildIDs = append([]uint64(nil), group.GuildIDs...)
	}
	return out
}

func hasU64(list []uint64, id uint64) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

func hasI64(list []int64, id int64) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

func toggleU64(list *[]uint64, id uint64) bool {
	for i, v := range *list {
		if v == id {
			*list = append((*list)[:i], (*list)[i+1:]...)
			return false
		}
	}
	*list = append(*list, id)
	return true
}

func toggleI64(list *[]int64, id int64) bool {
	for i, v := range *list {
		if v == id {
			*list = append((*list)[:i], (*list)[i+1:]...)
			return false
		}
	}
	*list = append(*list, id)
	return true
}
