package widget

// BottomScroll tracks a scroll offset measured from the newest content.
// Offset zero means the viewport is anchored to the bottom. When content grows
// while the user is reading older content, the offset grows by the same amount
// so the visible rows remain stable.
type BottomScroll struct {
	offset   int
	content  int
	viewport int
	known    bool
}

// Update records the current content and viewport sizes while preserving the
// reading position when content is appended below it.
func (s *BottomScroll) Update(content, viewport int) {
	if s == nil {
		return
	}
	content = maxInt(content, 0)
	viewport = maxInt(viewport, 0)
	if s.offset > 0 && content > s.content {
		s.offset += content - s.content
	}
	s.content = content
	s.viewport = viewport
	s.known = true
	s.offset = clampOffset(s.offset, s.maxOffset())
}

// UpdateAnchored records a content change while keeping the requested top row
// at the same position. Interactive lists use this when an explicitly focused
// item is a stronger anchor than the newest content.
func (s *BottomScroll) UpdateAnchored(content, viewport, top int) {
	if s == nil {
		return
	}
	content = maxInt(content, 0)
	viewport = maxInt(viewport, 0)
	top = maxInt(top, 0)
	s.content = content
	s.viewport = viewport
	s.known = true
	s.offset = clampOffset(content-viewport-top, s.maxOffset())
}

// UpdatePrepend records content that was added before the existing content.
// The bottom-relative offset must stay unchanged so the previously visible
// messages remain in place while older history becomes available above them.
func (s *BottomScroll) UpdatePrepend(content, viewport int) {
	if s == nil {
		return
	}
	s.content = maxInt(content, 0)
	s.viewport = maxInt(viewport, 0)
	s.known = true
	s.offset = clampOffset(s.offset, s.maxOffset())
}

// Offset returns the distance from the newest content.
func (s *BottomScroll) Offset() int {
	if s == nil {
		return 0
	}
	return s.offset
}

// SetOffset sets the distance from the newest content.
func (s *BottomScroll) SetOffset(offset int) {
	if s == nil {
		return
	}
	if s.known {
		s.offset = clampOffset(offset, s.maxOffset())
		return
	}
	s.offset = maxInt(offset, 0)
}

func clampOffset(offset, maxOffset int) int {
	if offset < 0 {
		return 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func (s *BottomScroll) maxOffset() int {
	return maxInt(s.content-s.viewport, 0)
}
