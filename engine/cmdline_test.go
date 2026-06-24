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
	if got := string(e.scr.colon); got != "\t\n" {
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

func TestColonCtrlHBackspace(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	drive(e, ":")
	e.Input(KeyEvent{Rune: 'a'})
	e.Input(KeyEvent{Rune: 'h', Mods: ModCtrl})
	if got := string(e.scr.colon); got != "" {
		t.Fatalf("colon after ^H = %q", got)
	}
}