package engine

const (
	promptMorePages = "Press any key to continue [q to quit]: "
	promptLastPage  = "Press any key to continue [: to enter more ex commands]: "
)

// pendingPageSize is how many output lines fit on one overlay page. The overlay
// is drawn at the bottom over the buffer with a "+=+=" divider above it and a
// continue-prompt below (nvi vs_msg), so the block needs the divider and prompt
// rows in addition to the text -- one page holds rows-1 lines.
func (s *screen) pendingPageSize() int {
	page := s.rows - 1
	if page < 1 {
		page = 1
	}
	return page
}

func (s *screen) pendingPageLines() []string {
	if s.pendingOutput == nil {
		return nil
	}
	page := s.pendingPageSize()
	start := s.pendingPage * page
	if start >= len(s.pendingOutput) {
		return nil
	}
	end := start + page
	if end > len(s.pendingOutput) {
		end = len(s.pendingOutput)
	}
	return s.pendingOutput[start:end]
}

func (s *screen) pendingHasMorePages() bool {
	if s.pendingOutput == nil {
		return false
	}
	return (s.pendingPage+1)*s.pendingPageSize() < len(s.pendingOutput)
}

// pendingOutputFirst reports whether the first overlay page is showing, so the
// frontend draws the divider only where the ex output begins (nvi vs_divider).
func (s *screen) pendingOutputFirst() bool { return s.pendingPage == 0 }

func (s *screen) pendingOutputPrompt() string {
	if s.pendingOutput == nil {
		return ""
	}
	if s.pendingHasMorePages() {
		return promptMorePages
	}
	return promptLastPage
}

func (e *Engine) clearPendingOutput() {
	e.scr.pendingOutput = nil
	e.scr.pendingPage = 0
}

// overlayKey extracts the rune to act on for pagination (first key of an event).
func overlayKey(ev Event) (rune, bool) {
	switch v := ev.(type) {
	case KeyEvent:
		if v.Rune != 0 {
			return v.Rune, true
		}
		switch v.Key {
		case KeyEnter:
			return '\n', true
		case KeyEscape:
			return 0x1b, true
		case KeyTab:
			return '\t', true
		case KeyBackspace:
			return 0x7f, true
		}
		return 0, true // any other key: advance/dismiss
	case StringEvent:
		for _, r := range v.Text {
			return r, true
		}
		return 0, false
	case InterruptEvent:
		return 0x03, true
	default:
		return 0, false
	}
}

// handlePendingOutput processes a key while a paged output overlay is active.
// Returns true if the event was consumed.
func (e *Engine) handlePendingOutput(ev Event) bool {
	if e.scr.pendingOutput == nil {
		return false
	}
	r, ok := overlayKey(ev)
	if !ok {
		return false
	}
	if e.scr.pendingHasMorePages() {
		if r == 'q' {
			e.clearPendingOutput()
		} else {
			e.scr.pendingPage++
		}
		e.fe.Render(view{e.scr}, ChangeSet{Full: true})
		return true
	}
	// Final page: dismiss; ':' opens the colon line (nvi SCROLL_W_EX).
	e.clearPendingOutput()
	if r == ':' {
		e.enterCmdline(':')
	}
	e.fe.Render(view{e.scr}, ChangeSet{Full: true})
	return true
}
