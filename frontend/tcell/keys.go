package tcell

import (
	tc "github.com/gdamore/tcell/v2"

	"govi/engine"
)

// translateKey converts a tcell key event into an engine input event.
func translateKey(ev *tc.EventKey) engine.Event {
	mods := translateMods(ev.Modifiers())

	// Ctrl-letter keys arrive as the dedicated KeyCtrlA..KeyCtrlZ codes; turn
	// them back into a letter rune carrying the Ctrl modifier so the engine's
	// key tables can match them uniformly.
	if k := ev.Key(); k >= tc.KeyCtrlA && k <= tc.KeyCtrlZ {
		// Exclude the keys tcell aliases to Ctrl codes but which the engine
		// treats as named keys (Enter=Ctrl-M, Tab=Ctrl-I, Backspace=Ctrl-H).
		switch k {
		case tc.KeyTab, tc.KeyEnter, tc.KeyBackspace:
			// fall through to the named-key handling below
		default:
			return engine.KeyEvent{Rune: rune('a' + (k - tc.KeyCtrlA)), Mods: mods | engine.ModCtrl}
		}
	}

	// Control keys outside the A-Z block that vi binds to commands.
	switch ev.Key() {
	case tc.KeyCtrlCarat: // ^^ : alternate file
		return engine.KeyEvent{Rune: '^', Mods: mods | engine.ModCtrl}
	case tc.KeyCtrlRightSq: // ^] : tag
		return engine.KeyEvent{Rune: ']', Mods: mods | engine.ModCtrl}
	case tc.KeyCtrlSpace: // ^@ / NUL
		return engine.KeyEvent{Rune: '@', Mods: mods | engine.ModCtrl}
	}

	switch ev.Key() {
	case tc.KeyRune:
		return engine.KeyEvent{Rune: ev.Rune(), Mods: mods}
	case tc.KeyEnter:
		return engine.KeyEvent{Key: engine.KeyEnter, Mods: mods}
	case tc.KeyTab:
		return engine.KeyEvent{Rune: '\t', Mods: mods}
	case tc.KeyEsc:
		return engine.KeyEvent{Key: engine.KeyEscape, Mods: mods}
	case tc.KeyBackspace, tc.KeyBackspace2:
		return engine.KeyEvent{Key: engine.KeyBackspace, Mods: mods}
	case tc.KeyDelete:
		return engine.KeyEvent{Key: engine.KeyDelete, Mods: mods}
	case tc.KeyUp:
		return engine.KeyEvent{Key: engine.KeyUp, Mods: mods}
	case tc.KeyDown:
		return engine.KeyEvent{Key: engine.KeyDown, Mods: mods}
	case tc.KeyLeft:
		return engine.KeyEvent{Key: engine.KeyLeft, Mods: mods}
	case tc.KeyRight:
		return engine.KeyEvent{Key: engine.KeyRight, Mods: mods}
	case tc.KeyHome:
		return engine.KeyEvent{Key: engine.KeyHome, Mods: mods}
	case tc.KeyEnd:
		return engine.KeyEvent{Key: engine.KeyEnd, Mods: mods}
	case tc.KeyPgUp:
		return engine.KeyEvent{Key: engine.KeyPageUp, Mods: mods}
	case tc.KeyPgDn:
		return engine.KeyEvent{Key: engine.KeyPageDown, Mods: mods}
	case tc.KeyInsert:
		return engine.KeyEvent{Key: engine.KeyInsert, Mods: mods}
	}

	if ev.Key() >= tc.KeyF1 && ev.Key() <= tc.KeyF64 {
		return engine.KeyEvent{Key: engine.KeyFunc, FuncN: int(ev.Key()-tc.KeyF1) + 1, Mods: mods}
	}

	// Remaining control codes (e.g. Esc variants) map to their rune.
	return engine.KeyEvent{Rune: ev.Rune(), Mods: mods}
}

func translateMods(m tc.ModMask) engine.Mod {
	var out engine.Mod
	if m&tc.ModCtrl != 0 {
		out |= engine.ModCtrl
	}
	if m&tc.ModAlt != 0 {
		out |= engine.ModAlt
	}
	if m&tc.ModShift != 0 {
		out |= engine.ModShift
	}
	return out
}
