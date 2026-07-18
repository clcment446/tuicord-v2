package ui

import "awesomeProject/internal/tui/text"

func chatLineWidth(line chatLine) int {
	width := 0
	for _, segment := range line.segments {
		width += text.Width(segment.text)
	}
	return width
}

func chatLineHasVisibleContent(line chatLine) bool {
	if line.media != nil || len(line.inlineMedia) > 0 {
		return true
	}
	if line.text != "" && text.Width(line.text) > 0 {
		return true
	}
	return chatLineWidth(line) > 0
}

// translateChatLine returns line shifted right by offset cells. It copies every
// slice that needs coordinate adjustment so cached source lines remain
// immutable when embeds and component containers decorate them.
func translateChatLine(line chatLine, offset int) chatLine {
	next := line
	next.mediaX += offset
	next.segments = append([]chatSegment(nil), line.segments...)
	if len(line.inlineMedia) > 0 {
		next.inlineMedia = append([]positionedInlineMedia(nil), line.inlineMedia...)
		for i := range next.inlineMedia {
			next.inlineMedia[i].col += offset
		}
	}
	if len(line.actions) > 0 {
		next.actions = append([]componentHit(nil), line.actions...)
		for i := range next.actions {
			next.actions[i].start += offset
			next.actions[i].end += offset
		}
	}
	if len(line.entities) > 0 {
		next.entities = append([]entityHit(nil), line.entities...)
		for i := range next.entities {
			next.entities[i].start += offset
			next.entities[i].end += offset
		}
	}
	return next
}
