package ui

import (
	"image"
	"sort"
	"strconv"
	"strings"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

type profileDM struct {
	ID   store.ChannelID
	Name string
}

// profileRole is a role chip on the profile card, colored like the role.
type profileRole struct {
	Name string
	// Color is the role's 0xRRGGBB color, or zero for an uncolored role.
	Color uint32
}

type profileDetails struct {
	ID        store.UserID
	Name      string
	Username  string
	Nick      string
	AvatarURL string
	Roles     []profileRole
	// Guilds lists servers this client shares with the user, best effort from
	// the local member caches.
	Guilds []string
	DMs    []profileDM
}

// ProfilePopup is a modal floating profile card. It owns pointer capture while
// mounted so dragging or selecting a DM never activates the view underneath.
type ProfilePopup struct {
	details  profileDetails
	styles   Styles
	onOpenDM func(store.ChannelID)
	onClose  func()
	node     layout.Node

	x, y             int
	screenW, screenH int
	dragging         bool
	dragStartX       int
	dragStartY       int
	originX          int
	originY          int
	dmRow            int
	selectedDM       int

	// avatar is the fetched profile picture, delivered asynchronously via
	// SetAvatar once the media fetcher resolves details.AvatarURL.
	avatar image.Image
}

// SetAvatar attaches the fetched profile picture. Must be called on the UI
// goroutine.
func (p *ProfilePopup) SetAvatar(img image.Image) {
	if p != nil {
		p.avatar = img
	}
}

// SetDetails replaces the card's identity and role data after an asynchronous
// member fetch, preserving the already-resolved avatar. Must be called on the
// UI goroutine.
func (p *ProfilePopup) SetDetails(details profileDetails) {
	if p != nil {
		p.details = details
	}
}

func NewProfilePopup(details profileDetails, styles Styles, onOpenDM func(store.ChannelID), onClose func()) *ProfilePopup {
	return &ProfilePopup{details: details, styles: styles, onOpenDM: onOpenDM, onClose: onClose, node: layout.Node{Grow: 1}, x: -1, y: -1}
}

func buildProfileDetails(st *store.Store, guild, dmGuild store.GuildID, id store.UserID) profileDetails {
	details := profileDetails{ID: id}
	if st == nil {
		return details
	}
	member, _ := st.Member(guild, id)
	details.Name = member.Name
	details.Username = member.Username
	details.Nick = member.Nick
	details.AvatarURL = member.AvatarURL
	type positionedRole struct {
		role     store.Role
		position int
	}
	var roles []positionedRole
	for _, roleID := range member.RoleIDs {
		if role, ok := st.Role(guild, roleID); ok {
			roles = append(roles, positionedRole{role: role, position: role.Position})
		}
	}
	// Discord orders role lists highest first.
	sort.SliceStable(roles, func(i, j int) bool { return roles[i].position > roles[j].position })
	for _, r := range roles {
		details.Roles = append(details.Roles, profileRole{Name: r.role.Name, Color: r.role.Color})
	}
	for _, g := range st.Guilds() {
		if g.Unavailable {
			continue
		}
		if m, ok := st.Member(g.ID, id); ok {
			if details.AvatarURL == "" {
				details.AvatarURL = m.AvatarURL
			}
			details.Guilds = append(details.Guilds, g.Name)
		}
	}
	for _, channel := range st.Channels(dmGuild) {
		for _, recipient := range channel.RecipientIDs {
			if recipient == id {
				details.DMs = append(details.DMs, profileDM{ID: channel.ID, Name: channel.Name})
				break
			}
		}
	}
	return details
}

func (p *ProfilePopup) CanFocus() bool       { return p != nil }
func (p *ProfilePopup) PreferredFocus() bool { return true }
func (p *ProfilePopup) Layout() *layout.Node { return &p.node }
func (p *ProfilePopup) Measure(avail tui.Size) tui.Size {
	p.screenW, p.screenH = avail.W, avail.H
	return avail
}

// maxProfileGuilds caps the common-server rows; the remainder collapses into a
// single "+N more" row.
const maxProfileGuilds = 5

func (p *ProfilePopup) guildRows() int {
	if len(p.details.Guilds) == 0 {
		return 0
	}
	// blank + "Servers" header + entries (capped, extra collapsed into one row)
	return 2 + min(len(p.details.Guilds), maxProfileGuilds+1)
}

func (p *ProfilePopup) dimensions(w, h int) (int, int) {
	width := min(52, max(30, w-4))
	roleRows := max(len(p.details.Roles), 1)
	height := 10 + roleRows + p.guildRows() + len(p.details.DMs)
	return min(width, max(w, 0)), min(height, max(h, 0))
}

func (p *ProfilePopup) box(w, h int) screen.Rect {
	bw, bh := p.dimensions(w, h)
	x, y := p.x, p.y
	if x < 0 {
		x = max((w-bw)/2, 0)
	}
	if y < 0 {
		y = max((h-bh)/2, 0)
	}
	x = min(max(x, 0), max(w-bw, 0))
	y = min(max(y, 0), max(h-bh, 0))
	return screen.Rect{X: x, Y: y, W: bw, H: bh}
}

func (p *ProfilePopup) Draw(r screen.Region) {
	if p == nil || r.Width() < 2 || r.Height() < 2 {
		return
	}
	p.screenW, p.screenH = r.Width(), r.Height()
	rect := p.box(r.Width(), r.Height())
	box := r.Clip(rect)
	base := p.styles.Cell("messages.content")
	border := p.styles.Cell("panels.border")
	title := p.styles.Cell("messages.author")
	muted := p.styles.Cell("muted")
	selected := p.styles.Cell("messages.focused")
	selected.Attrs |= screen.Reverse
	box.Fill(screen.Rect{W: rect.W, H: rect.H}, screen.Cell{Content: " ", Style: base})
	drawProfileBorder(box, rect.W, rect.H, border)
	drawPreviewText(box, 2, 0, " Profile · drag ", rect.W-4, title)
	name := p.details.Name
	if name == "" {
		name = p.details.Username
	}
	drawPreviewText(box, 2, 2, "Name: "+fallbackProfileValue(name), rect.W-4, base)
	drawPreviewText(box, 2, 3, "Username: "+fallbackProfileValue(p.details.Username), rect.W-4, base)
	drawPreviewText(box, 2, 4, "Server nick: "+fallbackProfileValue(p.details.Nick), rect.W-4, base)
	drawPreviewText(box, 2, 5, "ID: "+strconv.FormatUint(uint64(p.details.ID), 10), rect.W-4, muted)
	if p.avatar != nil {
		const avatarCols, avatarRows = 8, 4
		if rect.W > avatarCols+14 && rect.H > avatarRows+2 {
			// Keep the popup's terminal image separate from chat avatars. Closing
			// the popup frees its Kitty image ID; sharing the URL-derived chat ID
			// would also delete every chat placement backed by that image.
			img := widget.NewKittyImageFrom(p.avatar).
				SetID(stableImageID("profile:avatar:" + p.details.AvatarURL)).
				SetPlacementID(stableImageID("profile:avatar")).
				SetZ(-1).
				SetStyle(base)
			if b := p.avatar.Bounds(); b.Dx() > 0 && b.Dy() > 0 {
				img.SetPixelSize(b.Dx(), b.Dy())
			}
			img.Draw(box.Clip(screen.Rect{X: rect.W - avatarCols - 2, Y: 1, W: avatarCols, H: avatarRows}))
		}
	}
	row := 7
	drawPreviewText(box, 2, row, "Roles", rect.W-4, title)
	row++
	if len(p.details.Roles) == 0 {
		drawPreviewText(box, 3, row, "None", rect.W-5, muted)
		row++
	} else {
		for _, role := range p.details.Roles {
			style := base
			if role.Color != 0 {
				style.Fg = rgbColor(role.Color)
			}
			drawPreviewText(box, 3, row, "@"+role.Name, rect.W-5, style)
			row++
		}
	}
	if len(p.details.Guilds) > 0 {
		row++
		drawPreviewText(box, 2, row, "Servers in common", rect.W-4, title)
		row++
		shown := p.details.Guilds
		if len(shown) > maxProfileGuilds+1 {
			shown = shown[:maxProfileGuilds]
		}
		for _, name := range shown {
			drawPreviewText(box, 3, row, "· "+name, rect.W-5, base)
			row++
		}
		if extra := len(p.details.Guilds) - len(shown); extra > 0 {
			drawPreviewText(box, 3, row, "… +"+strconv.Itoa(extra)+" more", rect.W-5, muted)
			row++
		}
	}
	p.dmRow = row
	for i, dm := range p.details.DMs {
		style := base
		if i == p.selectedDM {
			style = mergeStyle(style, selected)
		}
		label := "Open DM"
		if dm.Name != "" {
			label += " · " + dm.Name
		}
		drawPreviewText(box, 2, row+i, label, rect.W-4, style)
	}
	drawPreviewText(box, max(rect.W-text.Width("Esc close")-2, 1), rect.H-1, "Esc close", rect.W-2, muted)
}

func drawProfileBorder(r screen.Region, w, h int, style screen.Style) {
	if w < 2 || h < 2 {
		return
	}
	for x := 0; x < w; x++ {
		r.Set(x, 0, screen.Cell{Content: "─", Style: style})
		r.Set(x, h-1, screen.Cell{Content: "─", Style: style})
	}
	for y := 0; y < h; y++ {
		r.Set(0, y, screen.Cell{Content: "│", Style: style})
		r.Set(w-1, y, screen.Cell{Content: "│", Style: style})
	}
	r.Set(0, 0, screen.Cell{Content: "╭", Style: style})
	r.Set(w-1, 0, screen.Cell{Content: "╮", Style: style})
	r.Set(0, h-1, screen.Cell{Content: "╰", Style: style})
	r.Set(w-1, h-1, screen.Cell{Content: "╯", Style: style})
}

func fallbackProfileValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	return value
}

func (p *ProfilePopup) Handle(ev tui.Event) bool {
	if p == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return true
		}
		switch ev.Key {
		case input.KeyEsc:
			p.close()
		case input.KeyUp:
			p.selectedDM = max(p.selectedDM-1, 0)
		case input.KeyDown:
			p.selectedDM = min(p.selectedDM+1, max(len(p.details.DMs)-1, 0))
		case input.KeyEnter:
			p.openSelectedDM()
		case input.KeyRune:
			if ev.Rune == 'u' || ev.Rune == 'U' {
				p.close()
			}
		}
		return true
	case input.MouseEvent:
		return p.handleMouse(ev)
	default:
		return true
	}
}

func (p *ProfilePopup) handleMouse(ev input.MouseEvent) bool {
	rect := p.box(p.screenW, p.screenH)
	switch ev.Kind {
	case input.MousePress:
		if ev.Btn == input.ButtonLeft && ev.Y == rect.Y && ev.X >= rect.X && ev.X < rect.X+rect.W {
			p.dragging = true
			p.dragStartX, p.dragStartY = ev.X, ev.Y
			p.originX, p.originY = rect.X, rect.Y
			return true
		}
		if ev.Btn == input.ButtonLeft && ev.X >= rect.X && ev.X < rect.X+rect.W {
			index := ev.Y - rect.Y - p.dmRow
			if index >= 0 && index < len(p.details.DMs) {
				p.selectedDM = index
				p.openSelectedDM()
			}
		}
	case input.MouseMotion:
		if p.dragging {
			p.x = p.originX + ev.X - p.dragStartX
			p.y = p.originY + ev.Y - p.dragStartY
		}
	case input.MouseRelease:
		p.dragging = false
	}
	return true
}

func (p *ProfilePopup) openSelectedDM() {
	if p.selectedDM >= 0 && p.selectedDM < len(p.details.DMs) && p.onOpenDM != nil {
		p.onOpenDM(p.details.DMs[p.selectedDM].ID)
	}
}

func (p *ProfilePopup) close() {
	if p.onClose != nil {
		p.onClose()
	}
}
