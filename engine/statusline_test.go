package engine

import (
	"strings"
	"testing"
)

func TestStatusLineRuler(t *testing.T) {
	e, _, _ := newTestEngine(t, "hello\n")
	e.Resize(5, 30)
	e.scr.cursor = Pos{Line: 1, Col: 2}
	e.scr.msg = ""
	if err := e.exExecute("set ruler"); err != nil {
		t.Fatal(err)
	}
	msg, _ := (view{e.scr}).Message()
	if !strings.Contains(strings.TrimSpace(msg), "1,3") {
		t.Fatalf("status = %q, want ruler 1,3", msg)
	}
}

func TestStatusLineShowmode(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(5, 30)
	e.scr.msg = ""
	if err := e.exExecute("set showmode"); err != nil {
		t.Fatal(err)
	}
	msg, _ := (view{e.scr}).Message()
	if !strings.Contains(msg, "Command") {
		t.Fatalf("status = %q, want Command", msg)
	}
	drive(e, "i")
	msg, _ = (view{e.scr}).Message()
	if !strings.Contains(msg, "Insert") {
		t.Fatalf("insert status = %q, want Insert", msg)
	}
	drive(e, "\x1b")
	msg, _ = (view{e.scr}).Message()
	if !strings.Contains(msg, "Command") {
		t.Fatalf("after insert status = %q, want Command", msg)
	}
}

func TestStatusLineShowmodeAppendChangeReplace(t *testing.T) {
	e, _, _ := newTestEngine(t, "ab\n")
	e.Resize(5, 40)
	e.exExecute("set showmode")
	e.scr.msg = ""

	drive(e, "a")
	if msg, _ := (view{e.scr}).Message(); !strings.Contains(msg, "Append") {
		t.Fatalf("a: %q", msg)
	}
	drive(e, "\x1b")

	drive(e, "cc")
	if msg, _ := (view{e.scr}).Message(); !strings.Contains(msg, "Change") {
		t.Fatalf("cc: %q", msg)
	}
	drive(e, "\x1b")

	drive(e, "R")
	if msg, _ := (view{e.scr}).Message(); !strings.Contains(msg, "Replace") {
		t.Fatalf("R: %q", msg)
	}
	drive(e, "\x1b")
}

func TestStatusLineModifiedFlag(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.Resize(5, 30)
	e.exExecute("set showmode")
	e.scr.msg = ""
	drive(e, "i!")
	drive(e, "\x1b")
	msg, _ := (view{e.scr}).Message()
	if !strings.Contains(msg, "*Command") {
		t.Fatalf("modified status = %q, want *Command", msg)
	}
}

func TestStatusLineMessageOverridesDefault(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	e.exExecute("set ruler showmode")
	e.scr.msg = "saved"
	e.scr.msgKind = MsgInfo
	msg, kind := (view{e.scr}).Message()
	if msg != "saved" || kind != MsgInfo {
		t.Fatalf("Message() = %q %v, want saved", msg, kind)
	}
}

func TestLayoutStatusLine(t *testing.T) {
	got := layoutStatusLine(40, "1,5", "*Insert")
	if !strings.Contains(got, "1,5") || !strings.Contains(got, "*Insert") {
		t.Fatalf("layout = %q", got)
	}
}