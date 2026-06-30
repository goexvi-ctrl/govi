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
	if !strings.HasPrefix(got, "Version govi-0.1") {
		t.Fatalf("VersionString = %q", got)
	}
}

func TestVersionStringDirtyBuildTime(t *testing.T) {
	oldDate, oldState, oldBuild := commitDate, treeState, buildTime
	defer func() {
		commitDate = oldDate
		treeState = oldState
		buildTime = oldBuild
	}()
	commitDate = "2026-06-25"
	treeState = "modified"
	buildTime = "2026-06-26T12:00:00Z"
	got := VersionString()
	if !strings.Contains(got, "(2026-06-25)") {
		t.Fatalf("VersionString = %q, want commit date", got)
	}
	if !strings.Contains(got, "modified") {
		t.Fatalf("VersionString = %q, want modified", got)
	}
	if !strings.Contains(got, buildTime) {
		t.Fatalf("VersionString = %q, want build timestamp", got)
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
