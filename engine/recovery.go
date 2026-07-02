package engine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"govi/engine/buffer"
)

// Crash recovery. While a buffer has unsaved changes, the engine keeps a
// recovery file in the recovery directory (the recdir option). If the editor or
// system dies, the changes can be recovered with `nvi -r file` or `:recover`.
// This corresponds to nvi's preserve/recover machinery (common/recover.c),
// though the on-disk format is govi's own.
//
// The recovery file is a text file beginning with a small header so it can be
// scanned and matched to its original file:
//
//	govi recovery
//	File: /abs/path/to/original
//	Time: <unix seconds>
//	Lines: <n>
//	<blank line>
//	<line 1>
//	<line 2>
//	...

// recoverInterval throttles automatic recovery syncs during editing.
const recoverInterval = 30 * time.Second

const recoverMagic = "govi recovery"

// recdir returns (creating if needed) the recovery directory.
func (e *Engine) recdir() string {
	dir := e.scr.opts.Str("recdir")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "vi.recover")
	}
	os.MkdirAll(dir, 0o700)
	return dir
}

// noteRecovery is called after each change to keep the recovery file current.
// The first change writes it immediately; later changes are throttled and
// otherwise flushed when the editor goes idle (see the frontend).
func (e *Engine) noteRecovery() {
	if e.scr.name == "" {
		return
	}
	e.recoverDirty = true
	if e.recoverPath == "" {
		e.syncRecovery(true)
		return
	}
	e.syncRecovery(false)
}

// NeedsRecoverySync reports that there are changes not yet in the recovery file,
// so a host can flush them with SyncRecovery after a brief idle.
func (e *Engine) NeedsRecoverySync() bool { return e.recoverDirty && e.scr.modified }

// SyncRecovery forces the recovery file up to date if the buffer is modified.
// Hosts may call it on a timer; :preserve calls it too.
func (e *Engine) SyncRecovery() { e.syncRecovery(true) }

func (e *Engine) syncRecovery(force bool) {
	if !e.scr.modified || e.scr.name == "" || !e.recoverDirty {
		return
	}
	if !force && time.Since(e.recoverSync) < recoverInterval {
		return
	}
	if e.recoverPath == "" {
		f, err := os.CreateTemp(e.recdir(), "recover."+filepath.Base(e.scr.name)+".")
		if err != nil {
			return
		}
		e.recoverPath = f.Name()
		f.Close()
	}
	e.writeRecovery(e.recoverPath)
	e.recoverSync = time.Now()
	e.recoverDirty = false
}

func (e *Engine) writeRecovery(path string) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rec-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	tmp.Chmod(0o600)
	// A large buffer keeps the per-line writes from turning into many small
	// write(2)s when snapshotting a big buffer.
	w := bufio.NewWriterSize(tmp, 64<<10)
	abs := e.resolvePath(e.scr.name) // resolve against the editor's cwd, not the process cwd
	n := e.scr.store.Lines()
	fmt.Fprintf(w, "%s\nFile: %s\nTime: %d\nLines: %d\n\n", recoverMagic, abs, time.Now().Unix(), n)
	for i := int64(1); i <= n; i++ {
		line, _ := e.scr.store.Get(i)
		// Write the runes straight to the buffer; WriteRune fast-paths ASCII to
		// a single byte, so this avoids allocating a string per line.
		for _, r := range line {
			w.WriteRune(r)
		}
		w.WriteByte('\n')
	}
	if w.Flush() != nil || tmp.Close() != nil {
		os.Remove(tmpName)
		return
	}
	os.Rename(tmpName, path)
}

// removeRecovery deletes this session's recovery file (after a clean save or
// exit). A :preserve'd file is detached instead of deleted -- the point of
// :preserve is that the snapshot survives for a later vi -r, as in nvi; any
// further changes then start a fresh recovery file.
func (e *Engine) removeRecovery() {
	if e.recoverPath != "" && !e.recoverKeep {
		os.Remove(e.recoverPath)
	}
	e.recoverPath = ""
	e.recoverDirty = false
	e.recoverKeep = false
}

// recoveredFile holds a parsed recovery file.
type recoveredFile struct {
	path  string // recovery file path
	orig  string // original file path
	mtime int64
	lines [][]rune
}

// parseRecovery reads and parses a recovery file.
func parseRecovery(path string) (*recoveredFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)
	if !strings.HasPrefix(text, recoverMagic) {
		return nil, fmt.Errorf("%s: not a recovery file", path)
	}
	rec := &recoveredFile{path: path}
	// Split header (up to the first blank line) from the body.
	hdrEnd := strings.Index(text, "\n\n")
	if hdrEnd < 0 {
		return nil, fmt.Errorf("%s: malformed recovery file", path)
	}
	for _, line := range strings.Split(text[:hdrEnd], "\n") {
		switch {
		case strings.HasPrefix(line, "File: "):
			rec.orig = strings.TrimPrefix(line, "File: ")
		case strings.HasPrefix(line, "Time: "):
			rec.mtime, _ = strconv.ParseInt(strings.TrimPrefix(line, "Time: "), 10, 64)
		}
	}
	body := text[hdrEnd+2:]
	if body != "" {
		parts := strings.Split(body, "\n")
		if strings.HasSuffix(body, "\n") {
			parts = parts[:len(parts)-1]
		}
		rec.lines = make([][]rune, len(parts))
		for i, p := range parts {
			rec.lines[i] = []rune(p)
		}
	}
	return rec, nil
}

// RecoveryEntry describes one recoverable file in the recovery directory.
type RecoveryEntry struct {
	Orig  string    // original file path
	Mtime time.Time // recovery file modification time
	Path  string    // recovery file path
}

// ListRecoverable scans recdir for recover.* files and returns those that parse
// as govi recovery files. This corresponds to nvi's rcv_list (common/recover.c).
func (e *Engine) ListRecoverable() ([]RecoveryEntry, error) {
	dir := e.recdir()
	matches, err := filepath.Glob(filepath.Join(dir, "recover.*"))
	if err != nil {
		return nil, err
	}
	var out []RecoveryEntry
	for _, m := range matches {
		rec, err := parseRecovery(m)
		if err != nil {
			continue
		}
		mt := time.Unix(rec.mtime, 0)
		if info, err := os.Stat(m); err == nil {
			mt = info.ModTime()
		}
		out = append(out, RecoveryEntry{Orig: rec.orig, Mtime: mt, Path: m})
	}
	return out, nil
}

// findRecovery returns the newest recovery file whose original path matches an
// absolute form of name, or whose own path is name.
func (e *Engine) findRecovery(name string) (*recoveredFile, error) {
	// name may itself be a recovery file.
	if rec, err := parseRecovery(name); err == nil {
		return rec, nil
	}
	abs, _ := filepath.Abs(name)
	dir := e.recdir()
	matches, _ := filepath.Glob(filepath.Join(dir, "recover.*"))
	var best *recoveredFile
	for _, m := range matches {
		rec, err := parseRecovery(m)
		if err != nil {
			continue
		}
		if rec.orig == abs && (best == nil || rec.mtime > best.mtime) {
			best = rec
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no recovery file for %s", name)
	}
	return best, nil
}

// hasRecovery reports whether a recovery file exists for name (used to warn on
// open).
func (e *Engine) hasRecovery(name string) bool {
	_, err := e.findRecovery(name)
	return err == nil
}

// Recover loads the recovered contents for name into the buffer, leaving it
// modified so the user can write it back to the original file.
func (e *Engine) Recover(name string) error {
	rec, err := e.findRecovery(name)
	if err != nil {
		return err
	}
	e.replaceBuffer(buffer.NewMemFromLines(rec.lines), rec.orig)
	e.scr.modified = true
	e.recoverPath = rec.path // keep editing the same recovery file
	e.recoverSync = time.Now()
	e.scr.msg = fmt.Sprintf("Recovered %q (%d lines); write to save", filepath.Base(rec.orig), len(rec.lines))
	e.scr.msgKind = MsgInfo
	return nil
}

// exPreserve implements :preserve.
func (e *Engine) exPreserve(c *exCmd) error {
	if e.scr.name == "" {
		return fmt.Errorf("No current filename to preserve")
	}
	if !e.scr.modified {
		e.scr.msg, e.scr.msgKind = "No changes to preserve", MsgInfo
		return nil
	}
	e.syncRecovery(true)
	e.recoverKeep = true
	e.scr.msg, e.scr.msgKind = "File preserved", MsgInfo
	return nil
}

// exRecover implements :recover [file].
func (e *Engine) exRecover(c *exCmd) error {
	name := strings.TrimSpace(c.arg)
	if name == "" {
		name = e.scr.name
	}
	if name == "" {
		return fmt.Errorf("recover: no filename")
	}
	if e.scr.modified && !c.force {
		return fmt.Errorf("No write since last change (use :recover! to override)")
	}
	return e.Recover(name)
}
