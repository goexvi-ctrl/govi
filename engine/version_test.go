package engine

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	got := VersionString()
	if got == "" {
		t.Fatal("VersionString returned empty")
	}
	if !strings.HasPrefix(got, "Version gnvi-0.1 (") {
		t.Fatalf("VersionString = %q", got)
	}
}

func TestExVersion(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("version"); err != nil {
		t.Fatal(err)
	}
	msg, kind := (view{e.scr}).Message()
	if kind != MsgInfo {
		t.Fatalf("kind = %v, want MsgInfo", kind)
	}
	if msg != VersionString() {
		t.Fatalf("message = %q, want %q", msg, VersionString())
	}
}

func TestExVersionAbbrev(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("ve"); err != nil {
		t.Fatal(err)
	}
	if msg, _ := (view{e.scr}).Message(); msg != VersionString() {
		t.Fatalf("ve: message = %q", msg)
	}
}

