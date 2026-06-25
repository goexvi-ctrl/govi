//go:build darwin

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// runGUI implements `govi -g`: hand the files to GoVi.app through a govi:// URL.
//
// A GUI app launched by open(1) inherits neither the shell's cwd nor a file
// list, and URLs have length/encoding limits. So the launcher writes a one-shot
// payload file (cwd, the files to open, the silent flag, and the -w FIFO) into a
// fixed, app-owned directory, then runs `open "govi://open?ctx=<token>"` naming
// only that payload. GoVi.app validates the token, reads the payload from the
// fixed location, deletes it, and treats it as pure data -- it only opens files,
// never writing or executing anything from the payload. LaunchServices routes
// the govi:// URL to GoVi.app (cold or already running); no bundle lookup needed.
func runGUI(silent, wait bool, files []string) int {
	if wait && len(files) == 0 {
		fmt.Fprintln(os.Stderr, "govi: -w requires at least one file")
		return 2
	}
	// GoVi.app runs as the user who owns the active graphical session and reads
	// its launch payload from that user's home dir. If we are a different user
	// (e.g. su'd to another account in a terminal), the app can never read what
	// we write, so refuse rather than silently open an empty window.
	if cuid, err := consoleUID(); err == nil {
		if me := os.Getuid(); uint32(me) != cuid {
			fmt.Fprintf(os.Stderr,
				"govi: -g must run as the user of the active graphical session (uid %d), but you are uid %d\n",
				cuid, me)
			return 1
		}
	}
	dir, err := launchPayloadDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "govi:", err)
		return 1
	}
	paths, err := resolveOpenPaths(files)
	if err != nil {
		fmt.Fprintln(os.Stderr, "govi:", err)
		return 1
	}
	if len(paths) == 0 {
		return 1
	}

	var fifo string
	if wait {
		fifo, err = makeWaitFifo()
		if err != nil {
			fmt.Fprintln(os.Stderr, "govi:", err)
			return 1
		}
	}

	token, err := writePayload(dir, silent, paths, fifo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "govi:", err)
		return 1
	}
	if err := exec.Command("open", "govi://open?ctx="+token).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "govi:", err)
		return 1
	}

	if wait {
		waitForFifo(fifo)
		os.Remove(fifo)
	}
	return 0
}

// consoleUID returns the uid of the user owning the active graphical session.
// On macOS the owner of /dev/console is the user logged in at the window server;
// with fast user switching only the *active* session owns it, so a backgrounded
// switched-out session is correctly not reported here. This is the uid GoVi.app
// will run as when launched via `open`.
func consoleUID() (uint32, error) {
	fi, err := os.Stat("/dev/console")
	if err != nil {
		return 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("cannot read /dev/console owner")
	}
	return st.Uid, nil
}

// launchPayloadDir is the fixed, app-owned directory GoVi.app validates payloads
// against, so a stray govi:// URL cannot point the app at an arbitrary file.
func launchPayloadDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Application Support", "GoVi", "launch")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// resolveOpenPaths makes each file absolute and creates any that don't exist
// (like vi). With no files it creates a temp vi.XXXXXX -- an empty editor GoVi.app
// discards when its window/tab closes.
func resolveOpenPaths(files []string) ([]string, error) {
	if len(files) == 0 {
		f, err := os.CreateTemp("", "vi.")
		if err != nil {
			return nil, err
		}
		name := f.Name()
		f.Close()
		return []string{name}, nil
	}
	cwd, _ := os.Getwd()
	var paths []string
	for _, f := range files {
		p := f
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		if _, err := os.Stat(p); err != nil {
			cf, err := os.OpenFile(p, os.O_CREATE, 0o644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "govi: cannot create %s\n", p)
				continue
			}
			cf.Close()
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// writePayload writes a one-shot launch payload (key=value lines; file= repeats)
// and returns its token, the base filename the govi:// URL references.
func writePayload(dir string, silent bool, paths []string, fifo string) (string, error) {
	f, err := os.CreateTemp(dir, "ctx-")
	if err != nil {
		return "", err
	}
	defer f.Close()
	var b strings.Builder
	if cwd, err := os.Getwd(); err == nil {
		fmt.Fprintf(&b, "cwd=%s\n", cwd)
	}
	if silent {
		b.WriteString("silent=1\n")
	}
	for _, p := range paths {
		fmt.Fprintf(&b, "file=%s\n", p)
	}
	if fifo != "" {
		fmt.Fprintf(&b, "fifo=%s\n", fifo)
	}
	if _, err := f.WriteString(b.String()); err != nil {
		return "", err
	}
	return filepath.Base(f.Name()), nil
}

// makeWaitFifo creates a FIFO for -w; its path travels in the payload.
func makeWaitFifo() (string, error) {
	f, err := os.CreateTemp("", "govwait.*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	f.Close()
	os.Remove(path)
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// waitForFifo blocks until GoVi.app opens the write end and signals completion.
func waitForFifo(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = bufio.NewReader(f).ReadString('\n')
}
