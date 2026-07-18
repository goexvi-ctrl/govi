package main

// Tooltips (GoVi.app). The engine carries the exrc-settable options (tooltip,
// tooltipdelay, tooltipfile -- see engine/tooltip.go); the tips package parses
// the tooltip file; and these exports let the host poll the mode/delay and
// resolve the word under a screen cell to its tooltip text. The Swift side owns
// the presentation (hover timer, tooltip panel, Command-click, context menu).

import "C"

import (
	"govi/engine"
	"govi/frontend/grid"
)

// GoviTooltipMode returns this editor's tooltip mode: 0=off, 1=hover (show
// after the mouse rests on a known word for GoviTooltipDelayMS), 2=manual
// (only on Command-click or the context menu; hover triggers also work in
// hover mode).
//
//export GoviTooltipMode
func GoviTooltipMode(h C.longlong) C.int {
	in := get(h)
	if in == nil {
		return 0
	}
	switch in.eng.StrOption("tooltip") {
	case "hover":
		return 1
	case "manual":
		return 2
	default:
		return 0
	}
}

// GoviTooltipDelayMS returns the hover delay in milliseconds (tooltipdelay,
// clamped to >= 0).
//
//export GoviTooltipDelayMS
func GoviTooltipDelayMS(h C.longlong) C.int {
	in := get(h)
	if in == nil {
		return 0
	}
	d := in.eng.IntOption("tooltipdelay")
	if d < 0 {
		d = 0
	}
	return C.int(d)
}

// GoviTooltipAt returns the tooltip text for the word under screen cell
// (x, y), or "" when the cell is not on active-pane buffer text or the word
// has no entry in the tooltipfile (malloc'd; caller frees). On a hit the
// word's caret span [l1:c1, l2:c2) is written so the host can anchor the
// tooltip at the word start and keep it up while the pointer stays inside the
// word. The tooltipfile is (re)loaded here when its path or content changed.
//
//export GoviTooltipAt
func GoviTooltipAt(h C.longlong, x, y C.int, l1 *C.longlong, c1 *C.int, l2 *C.longlong, c2 *C.int) *C.char {
	in := get(h)
	if in == nil {
		return C.CString("")
	}
	tbl := in.tips.Get(in.eng.StrOption("tooltipfile"))
	if len(tbl) == 0 {
		return C.CString("")
	}
	text := ""
	in.eng.WithView(func(v engine.View) {
		p, ok := grid.ScreenToBufferActive(v, int(x), int(y))
		if !ok {
			return
		}
		a, b := in.eng.WordRange(p.Line, p.Col)
		if a == b {
			return
		}
		tip, ok := tbl[in.eng.RangeText(a, b)]
		if !ok {
			return
		}
		text = tip
		*l1, *c1 = C.longlong(a.Line), C.int(a.Col)
		*l2, *c2 = C.longlong(b.Line), C.int(b.Col)
	})
	return C.CString(text)
}
