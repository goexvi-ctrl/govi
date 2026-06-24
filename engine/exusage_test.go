package engine

import (
	"strings"
	"testing"
)

func TestExUsageCoverage(t *testing.T) {
	for _, d := range exCmds {
		if _, ok := exCmdMeta[d.full]; !ok {
			t.Errorf("exCmdMeta missing entry for %q", d.full)
			continue
		}
		if d.summary == "" || d.usage == "" {
			t.Errorf("command %q missing summary/usage after init", d.full)
		}
	}
	for name := range exCmdMeta {
		found := false
		for _, d := range exCmds {
			if d.full == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("exCmdMeta[%q] has no matching exCmds entry", name)
		}
	}
}

func TestExHelp(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("help"); err != nil {
		t.Fatal(err)
	}
	out := e.scr.pendingOutput
	if len(out) < 4 {
		t.Fatalf("help output = %v", out)
	}
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "viusage") || !strings.Contains(joined, "exusage") {
		t.Fatalf("help missing usage hints: %q", joined)
	}
}

func TestExUsageList(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("exusage"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(e.scr.pendingOutput, "\n")
	for _, name := range []string{"delete:", "write:", "help:", "file:"} {
		if !strings.Contains(joined, name) {
			t.Errorf("exusage list missing %q\n%s", name, joined)
		}
	}
}

func TestExUsageCommand(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("exusage write"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(e.scr.pendingOutput, "\n")
	if !strings.Contains(joined, "write the buffer") {
		t.Fatalf("exusage write = %q", joined)
	}
}

func TestViUsageList(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("viusage"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(e.scr.pendingOutput, "\n")
	for _, key := range []string{"^J ^N j", "ZZ", "insert mode", "ex command line"} {
		if !strings.Contains(joined, key) {
			t.Errorf("viusage list missing %q\n%s", key, joined)
		}
	}
}

func TestViUsageKey(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("viusage j"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(e.scr.pendingOutput, "\n")
	if !strings.Contains(joined, "move down") {
		t.Fatalf("viusage j = %q", joined)
	}
}

func TestViUsageUnknownKey(t *testing.T) {
	e, _, _ := newTestEngine(t, "x\n")
	if err := e.exExecute("viusage q"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}
