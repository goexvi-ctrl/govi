//go:build darwin

package main

import "testing"

func TestRun_guiWaitRequiresFiles(t *testing.T) {
	code, _, _ := captureRun(t, []string{"-G"}, nil)
	if code != 2 {
		t.Fatalf("run(-G) = %d, want 2", code)
	}
}
