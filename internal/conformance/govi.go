package conformance

import (
	"os"
	"path/filepath"

	"govi/engine"
)

// nopFrontend is a Frontend that renders nothing; it lets the engine run fully
// headless for conformance comparison.
type nopFrontend struct{}

func (nopFrontend) Render(engine.View, engine.ChangeSet) {}
func (nopFrontend) Bell()                                {}
func (nopFrontend) SetTitle(string)                      {}

// RunGoviVi runs the same vi session against the govi engine and returns the
// resulting file contents, driving the engine exactly as a frontend would.
func RunGoviVi(s ViSession) (string, error) {
	dir, err := os.MkdirTemp("", "govi-self-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "buf.txt")
	if err := os.WriteFile(file, []byte(s.Input), 0o644); err != nil {
		return "", err
	}

	eng := engine.New(nopFrontend{}, engine.Options{})
	if err := eng.Open(file); err != nil {
		return "", err
	}
	eng.Resize(23, 80) // 24-row terminal, one status line
	// Keep recovery files in this per-test temp dir (removed above on return),
	// mirroring the local oracle, so a run never writes to the shared system
	// recovery dir even if it errors out before the closing :wq.
	eng.RunEx("set recdir=" + dir)

	feedKeys(eng, s.Keys)
	// Flush and quit: leave any insert mode, then :wq.
	feedKeys(eng, "\x1b:wq\r")

	out, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// RunGoviEx runs an ex session against the govi engine by typing each command
// on the colon line, then writing and quitting. It returns the file contents.
func RunGoviEx(s ExSession) (string, error) {
	dir, err := os.MkdirTemp("", "govi-self-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "buf.txt")
	if err := os.WriteFile(file, []byte(s.Input), 0o644); err != nil {
		return "", err
	}

	eng := engine.New(nopFrontend{}, engine.Options{})
	if err := eng.Open(file); err != nil {
		return "", err
	}
	eng.Resize(23, 80)
	eng.RunEx("set recdir=" + dir) // keep recovery local to this temp dir (see RunGoviVi)

	for _, cmd := range s.Commands {
		feedKeys(eng, ":"+cmd+"\r")
	}
	feedKeys(eng, ":wq\r")

	out, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// feedKeys translates a raw keystroke script into engine events.
func feedKeys(eng *engine.Engine, keys string) {
	for _, r := range keys {
		switch r {
		case '\x1b':
			eng.Input(engine.KeyEvent{Key: engine.KeyEscape})
		case '\r', '\n':
			eng.Input(engine.KeyEvent{Key: engine.KeyEnter})
		case '\x7f', '\b':
			eng.Input(engine.KeyEvent{Key: engine.KeyBackspace})
		default:
			eng.Input(engine.KeyEvent{Rune: r})
		}
	}
}
