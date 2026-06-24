package engine

import "testing"

func TestColonCtrlUKillLine(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 's', Mods: 0})
	e.Input(KeyEvent{Rune: 'e', Mods: 0})
	e.Input(KeyEvent{Rune: 't', Mods: 0})
	e.Input(KeyEvent{Rune: 'u', Mods: ModCtrl})
	if got := string(e.scr.colon); got != "" {
		t.Fatalf("colon after ^U = %q, want empty", got)
	}
	if e.scr.mode != ModeExColon {
		t.Fatalf("mode = %v, want colon", e.scr.mode)
	}
}

func TestColonCtrlVLiteral(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'v', Mods: ModCtrl})
	e.Input(KeyEvent{Rune: '\t'})
	if got := string(e.scr.colon); got != "\t" {
		t.Fatalf("colon after ^V tab = %q", got)
	}
	e.Input(KeyEvent{Rune: 'v', Mods: ModCtrl})
	e.Input(KeyEvent{Key: KeyEnter})
	if got := string(e.scr.colon); got != "\t\r" {
		t.Fatalf("colon after ^V enter = %q", got)
	}
}

func TestColonCtrlVLiteralRaw(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 0x16}) // ^V without ModCtrl
	e.Input(KeyEvent{Rune: 'x'})
	if got := string(e.scr.colon); got != "x" {
		t.Fatalf("colon = %q", got)
	}
}

func TestColonCtrlWWordErase(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	for _, r := range "set number" {
		e.Input(KeyEvent{Rune: r})
	}
	e.Input(KeyEvent{Rune: 'w', Mods: ModCtrl})
	if got := string(e.scr.colon); got != "set " {
		t.Fatalf("colon after ^W = %q, want %q", got, "set ")
	}
}

func TestColonDisplayControlChar(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'v', Mods: ModCtrl})
	e.Input(KeyEvent{Key: KeyEnter})
	msg, _ := (view{e.scr}).Message()
	if msg != ":^M" {
		t.Fatalf("msg = %q, want :^M", msg)
	}
}

func TestColonDisplayLiteralNextCaret(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'v', Mods: ModCtrl})
	msg, _ := (view{e.scr}).Message()
	if msg != ":^" {
		t.Fatalf("msg = %q, want :^", msg)
	}
}

func TestColonCtrlCInterrupt(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'w'})
	e.Input(KeyEvent{Rune: 'c', Mods: ModCtrl})
	if e.scr.mode != ModeCommand {
		t.Fatalf("mode = %v, want command after ^C", e.scr.mode)
	}
	if len(e.scr.colon) != 0 {
		t.Fatalf("colon = %q, want empty", string(e.scr.colon))
	}
}

func TestColonCtrlAInsertsControl(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'a', Mods: ModCtrl})
	if len(e.scr.colon) != 1 || e.scr.colon[0] != 1 {
		t.Fatalf("colon = %v, want [SOH]", e.scr.colon)
	}
	msg, _ := (view{e.scr}).Message()
	if msg != ":^A" {
		t.Fatalf("msg = %q, want :^A", msg)
	}
}

func TestColonRawCtrlB(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 0x02}) // ^B as a tty C0 byte (GUI path)
	if len(e.scr.colon) != 1 || e.scr.colon[0] != 2 {
		t.Fatalf("colon = %v, want [STX]", e.scr.colon)
	}
	msg, _ := (view{e.scr}).Message()
	if msg != ":^B" {
		t.Fatalf("msg = %q, want :^B", msg)
	}
}

func TestColonRawCtrlXHex(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 0x18}) // ^X
	e.Input(KeyEvent{Rune: '4'})
	e.Input(KeyEvent{Rune: '1'})
	e.Input(KeyEvent{Rune: ' '}) // non-hex terminates hex entry
	if len(e.scr.colon) < 1 || e.scr.colon[0] != 'A' {
		t.Fatalf("colon = %v, want leading A", e.scr.colon)
	}
}

func TestColonUmlautNFC(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	for _, r := range "r " {
		e.Input(KeyEvent{Rune: r})
	}
	// macOS dead-key composition can arrive decomposed before NFC.
	e.Input(KeyEvent{Rune: 'u'})
	e.Input(KeyEvent{Rune: '\u0308'})
	if len(e.scr.colon) != 3 || e.scr.colon[2] != '\u00fc' {
		t.Fatalf("colon = %v, want NFC ü", e.scr.colon)
	}
	msg, _ := (view{e.scr}).Message()
	if got := DisplayStringColumns(msg, 8); got != 4 {
		t.Fatalf("display cols = %d, want 4 (msg %q)", got, msg)
	}
}

func TestColonCtrlHBackspace(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'a'})
	e.Input(KeyEvent{Rune: 'h', Mods: ModCtrl})
	if got := string(e.scr.colon); got != "" {
		t.Fatalf("colon after ^H = %q", got)
	}
}
