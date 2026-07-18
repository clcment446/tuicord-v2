// Package uistate persists per-user, per-client view preferences that are not
// part of Discord's account settings: which guilds and channels the user has
// pinned locally, and which folders and categories they have collapsed.
//
// It is deliberately separate from package config. config.toml is hand-editable
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

// State is the persisted set of client-side view preferences. Guild and channel
// IDs are Discord snowflakes; folder IDs are the (possibly small) integers
// Discord assigns guild folders. The zero value is a valid, empty state.
type State struct {
	PinnedGuilds        []uint64 `toml:"pinned_guilds"`
	PinnedChannels      []uint64 `toml:"pinned_channels"`
	CollapsedFolders    []int64  `toml:"collapsed_folders"`
	CollapsedCategories []uint64 `toml:"collapsed_categories"`
	RecentStickers      []uint64 `toml:"recent_stickers"`
	FavoriteEmojis      []string `toml:"favorite_emojis"`
	FavoriteStickers    []uint64 `toml:"favorite_stickers"`
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
	return atomicfile.Write(path, 0o600, func(w io.Writer) error {
		return toml.NewEncoder(w).Encode(s)
	})
}

// TogglePinnedGuild flips a guild's pinned status and returns the new value.
func (s *State) TogglePinnedGuild(id uint64) bool { return toggleU64(&s.PinnedGuilds, id) }

// TogglePinnedChannel flips a channel's pinned status and returns the new value.
func (s *State) TogglePinnedChannel(id uint64) bool { return toggleU64(&s.PinnedChannels, id) }

// ToggleCollapsedFolder flips a folder's collapsed status and returns the new value.
func (s *State) ToggleCollapsedFolder(id int64) bool { return toggleI64(&s.CollapsedFolders, id) }

// ToggleCollapsedCategory flips a category's collapsed status and returns the new value.
func (s *State) ToggleCollapsedCategory(id uint64) bool {
	return toggleU64(&s.CollapsedCategories, id)
}

// IsPinnedGuild reports whether a guild is pinned.
func (s *State) IsPinnedGuild(id uint64) bool { return hasU64(s.PinnedGuilds, id) }

// IsPinnedChannel reports whether a channel is pinned.
func (s *State) IsPinnedChannel(id uint64) bool { return hasU64(s.PinnedChannels, id) }

// IsFolderCollapsed reports whether a folder is collapsed.
func (s *State) IsFolderCollapsed(id int64) bool { return hasI64(s.CollapsedFolders, id) }

// IsCategoryCollapsed reports whether a category is collapsed.
func (s *State) IsCategoryCollapsed(id uint64) bool { return hasU64(s.CollapsedCategories, id) }

// CollapsedFolderSet returns the collapsed folders as a set for store.OrderGuilds.
func (s *State) CollapsedFolderSet() map[int64]bool {
	out := make(map[int64]bool, len(s.CollapsedFolders))
	for _, id := range s.CollapsedFolders {
		out[id] = true
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
