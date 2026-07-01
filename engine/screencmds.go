package engine

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Background-screen and resize commands: :bg, :fg/:Fg, :resize, :display screens
// (nvi ex/ex_screen.c, ex/ex_display.c, vi/vs_split.c vs_fg/vs_bg/vs_swap/
// vs_resize).

// sortScreens re-sorts the displayed-screen list into display order and keeps
// e.cur pointing at the active screen.
func (e *Engine) sortScreens() {
	active := e.scr
	sort.SliceStable(e.screens, func(i, j int) bool {
		a, b := e.screens[i], e.screens[j]
		if a.roff != b.roff {
			return a.roff < b.roff
		}
		return a.coff < b.coff
	})
	for i, s := range e.screens {
		if s == active {
			e.cur = i
			break
		}
	}
	e.scr = e.screens[e.cur]
}

// takeBg removes and returns a background screen: the first one when name is
// empty, otherwise the one whose file name matches in full or by last component
// (nvi vs_getbg). Returns nil when there is no match.
func (e *Engine) takeBg(name string) *screen {
	idx := -1
	switch {
	case name == "":
		if len(e.bg) > 0 {
			idx = 0
		}
	default:
		for i, s := range e.bg {
			if s.name == name {
				idx = i
				break
			}
		}
		if idx < 0 {
			for i, s := range e.bg {
				if filepath.Base(s.name) == name {
					idx = i
					break
				}
			}
		}
	}
	if idx < 0 {
		return nil
	}
	s := e.bg[idx]
	e.bg = append(e.bg[:idx], e.bg[idx+1:]...)
	return s
}

// exBg implements :bg -- background the current screen and switch to the next
// displayed one (nvi vs_bg). It is refused when only one screen is displayed.
func (e *Engine) exBg(c *exCmd) error {
	if len(e.screens) <= 1 {
		return fmt.Errorf("You may not background your only displayed screen")
	}
	e.discardCurrentScreen(true)
	return nil
}

// exFg implements :fg [file] and :Fg [file]. :fg swaps the current screen with a
// background screen, taking over its geometry and sending the current screen to
// the background (nvi vs_swap). :Fg brings a background screen back as a new
// horizontal split (nvi vs_fg with E_NEWSCREEN).
func (e *Engine) exFg(c *exCmd) error {
	name := strings.TrimSpace(c.arg)
	if c.newScreen {
		return e.fgNewScreen(name)
	}
	nsp := e.takeBg(name)
	if nsp == nil {
		return e.noBgError(name)
	}
	s := e.scr
	// The background screen takes over the current screen's geometry.
	nsp.roff, nsp.coff, nsp.rows, nsp.cols = s.roff, s.coff, s.rows, s.cols
	nsp.mapRows = nsp.rows
	nsp.minMapRows = nsp.rows
	nsp.defScroll = 0
	for i, sc := range e.screens {
		if sc == s {
			e.screens[i] = nsp
			break
		}
	}
	e.bg = append(e.bg, s) // the displaced screen goes to the background
	e.scr = nsp
	e.sortScreens()
	nsp.clampCursor()
	nsp.scrollToCursor()
	e.setStatusMsg(nsp)
	return nil
}

// fgNewScreen brings a background screen back as a new horizontal split.
func (e *Engine) fgNewScreen(name string) error {
	nsp := e.takeBg(name)
	if nsp == nil {
		return e.noBgError(name)
	}
	parent := e.scr
	if err := e.splitHoriz(nsp); err != nil {
		e.bg = append(e.bg, nsp) // hook it back on failure (nvi)
		return err
	}
	e.setStatusMsg(parent)
	e.setStatusMsg(nsp)
	return nil
}

func (e *Engine) noBgError(name string) error {
	if name == "" {
		return fmt.Errorf("There are no background screens")
	}
	return fmt.Errorf("There's no background screen editing a file named %s", name)
}

// exResize implements :resize [+-]rows (nvi vs_resize). A bare count sets the
// absolute text-row height; a signed count grows (+) or shrinks (-) the screen,
// taking rows from a vertically adjacent screen.
func (e *Engine) exResize(c *exCmd) error {
	arg := strings.TrimSpace(c.arg)
	if arg == "" {
		return c.usageError()
	}
	signed := arg[0] == '+' || arg[0] == '-'
	n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(arg, "+"), "-"))
	if err != nil || n < 0 {
		return c.usageError()
	}
	switch {
	case !signed:
		return e.resizeScreen(n, aSet)
	case arg[0] == '+':
		return e.resizeScreen(n, aIncrease)
	default:
		return e.resizeScreen(n, aDecrease)
	}
}

type resizeAdj int

const (
	aSet resizeAdj = iota
	aIncrease
	aDecrease
)

// resizeScreen changes the active screen's height by count text rows, taking the
// rows from (or giving them to) a vertically adjacent screen that shares its
// column span (nvi vs_resize). A_SET converts to the equivalent grow/shrink.
func (e *Engine) resizeScreen(count int, adj resizeAdj) error {
	s := e.scr
	if count == 0 {
		return nil
	}
	if adj == aSet {
		switch {
		case s.rows == count:
			return nil
		case s.rows > count:
			adj, count = aDecrease, s.rows-count
		default:
			adj, count = aIncrease, count-s.rows
		}
	}
	above, below := e.horizNeighbors(s)
	var grow, shrink *screen
	if adj == aIncrease {
		grow = s
		switch {
		case below != nil && below.rows >= minScreenRows+count:
			shrink = below
			below.roff += count
			below.rows -= count
			grow.rows += count
		case above != nil && above.rows >= minScreenRows+count:
			shrink = above
			above.rows -= count
			grow.roff -= count
			grow.rows += count
		default:
			return fmt.Errorf("The screen cannot grow")
		}
	} else { // aDecrease
		if s.rows < minScreenRows+count {
			return fmt.Errorf("The screen can only shrink to %d rows", minScreenRows)
		}
		shrink = s
		switch {
		case above != nil:
			grow = above
			s.roff += count
			s.rows -= count
			above.rows += count
		case below != nil:
			grow = below
			s.rows -= count
			below.roff -= count
			below.rows += count
		default:
			return fmt.Errorf("The screen cannot shrink")
		}
	}
	for _, sc := range []*screen{grow, shrink} {
		sc.mapRows = sc.rows
		sc.minMapRows = sc.rows
		sc.defScroll = 0
		sc.clampCursor()
		sc.scrollToCursor()
		e.setStatusMsg(sc) // nvi sets SC_STATUS on both resized screens
	}
	return nil
}

// minScreenRows mirrors nvi's MINIMUM_SCREEN_ROWS.
const minScreenRows = 1

// exDisplay implements :display b[uffers] | s[creens] (nvi ex_display.c). Only
// the screens list is presented as a screen overlay; buffers fall back to the
// cut-buffer display.
func (e *Engine) exDisplay(c *exCmd) error {
	const usage = "Usage: display b[uffers] | c[onnections] | s[creens] | t[ags]"
	arg := strings.TrimSpace(c.arg)
	switch {
	case arg == "":
		return fmt.Errorf("%s", usage)
	case strings.HasPrefix("screens", arg):
		return e.displayScreens()
	case strings.HasPrefix("buffers", arg):
		return e.displayBuffers()
	case strings.HasPrefix("connections", arg):
		return e.cscopeDisplay()
	case strings.HasPrefix("tags", arg):
		return e.displayTags()
	default:
		return fmt.Errorf("%s", usage)
	}
}

// displayTags implements :display t[ags] (nvi ex_tag_display): the tag stack,
// most recent first, numbered from 1, showing the file each tag jump landed in
// and the tag (or cscope pattern) searched for. The current entry -- the most
// recent jump, where the cursor sits now -- is marked with '*'.
//
// govi stores the location saved *before* each jump, so the file a jump landed
// in is the file saved by the next jump; the newest jump's target is the current
// file.
func (e *Engine) displayTags() error {
	s := e.scr
	if len(s.tagStack) == 0 {
		s.msg, s.msgKind = "The tags stack is empty", MsgInfo
		return nil
	}
	var lines []string
	for i := len(s.tagStack) - 1; i >= 0; i-- {
		current := i == len(s.tagStack)-1
		dest := s.name
		if !current {
			dest = s.tagStack[i+1].file
		}
		if dest == "" {
			dest = "[No file]"
		}
		num := len(s.tagStack) - i
		lines = append(lines, formatTagLine(num, dest, s.tagStack[i].tag, current))
	}
	e.showOutput(lines)
	return nil
}

// formatTagLine renders one :display tags row, mirroring nvi's column layout: a
// right-justified file name (long names truncated to their tail after "...")
// followed by an optional '*' current marker and the tag name.
func formatTagLine(num int, file, tag string, current bool) string {
	const nameW = 30
	var b strings.Builder
	fmt.Fprintf(&b, "%2d ", num)
	if r := []rune(file); len(r) > nameW {
		fmt.Fprintf(&b, "   ... %s", string(r[len(r)-(nameW-4):]))
	} else {
		fmt.Fprintf(&b, "   %*s", nameW, file)
	}
	if current {
		b.WriteByte('*')
	}
	if tag != "" {
		if current {
			b.WriteString("    ")
		} else {
			b.WriteString("     ")
		}
		b.WriteString(tag)
	}
	return b.String()
}

// displayScreens lists the background screens (nvi ex_sdisplay): names separated
// by spaces, breaking to a new line before a name that would reach the screen
// width, with each over-long name wrapped to the terminal width.
func (e *Engine) displayScreens() error {
	if len(e.bg) == 0 {
		e.scr.msg, e.scr.msgKind = "No background screens to display", MsgInfo
		return nil
	}
	cols := e.scr.statusCols()
	var sb strings.Builder
	col, sep := 0, 0
	for cnt, s := range e.bg {
		name := s.name
		if name == "" {
			name = "[No file]"
		}
		ln := len(name) + sep
		col += ln
		if col >= cols-1 {
			col, sep = ln, 0
			sb.WriteByte('\n')
		} else if cnt != 0 {
			sep = 1
			sb.WriteByte(' ')
		}
		sb.WriteString(name)
	}
	var lines []string
	for _, ln := range strings.Split(sb.String(), "\n") {
		lines = append(lines, wrapToWidth(ln, cols)...)
	}
	e.showOutput(lines)
	return nil
}

// wrapToWidth splits s into chunks of at most width columns (terminal wrap),
// returning a single empty line for an empty string.
func wrapToWidth(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	r := []rune(s)
	if len(r) == 0 {
		return []string{""}
	}
	var out []string
	for len(r) > width {
		out = append(out, string(r[:width]))
		r = r[width:]
	}
	return append(out, string(r))
}

// displayBuffers lists the named cut buffers and their first line (a compact
// form of nvi's buffer display).
func (e *Engine) displayBuffers() error {
	var lines []string
	for r := 'a'; r <= 'z'; r++ {
		t := e.scr.regs.Get(r)
		if t.Empty() {
			continue
		}
		first := ""
		if len(t.Lines) > 0 {
			first = string(t.Lines[0])
		}
		lines = append(lines, fmt.Sprintf("%c  %s", r, first))
	}
	if len(lines) == 0 {
		e.scr.msg, e.scr.msgKind = "No cut buffers to display", MsgInfo
		return nil
	}
	e.showOutput(lines)
	return nil
}
