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

// runGUI implements `govi -g`: open the given files in GoVi.app, behaving like
// a command-line launcher. It mirrors nvi startup resolution and writes a
// launch context that GoVi.app reads, because a GUI app started via open(1)
// does not inherit the shell's cwd or EXINIT/NEXINIT. Returns a process exit
// code.
//
// This is the Go port of the former gui/govi shell script.
func runGUI(silent, wait bool, files []string) int {
	if wait && len(files) == 0 {
		fmt.Fprintln(os.Stderr, "govi: -w requires at least one file")
		return 2
	}

	app, err := findGoviApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "govi:", err)
		return 1
	}

	supportDir, err := goviSupportDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "govi:", err)
		return 1
	}
	if err := writeLaunchContext(supportDir, silent); err != nil {
		fmt.Fprintln(os.Stderr, "govi:", err)
		return 1
	}

	// No files: ask the app (even one already running) for a fresh empty editor
	// by opening a unique sentinel file under the support "new" dir. `open -a`
	// routes it through the open-documents event, which a running app reliably
	// receives (unlike a plain activation); the app recognizes the sentinel,
	// opens an empty buffer, and deletes it. The unique name keeps macOS from
	// skipping it as an already-open path.
	if len(files) == 0 {
		// Open an empty editor the way nvi does: create a temp file (vi.XXXXXX)
		// and open it; the app deletes it when its window/tab closes. It lives in
		// the temp dir because LaunchServices won't open documents under ~/Library,
		// and the unique name avoids macOS skipping an already-open path.
		f, err := os.CreateTemp("", "vi.")
		if err != nil {
			fmt.Fprintln(os.Stderr, "govi:", err)
			return 1
		}
		name := f.Name()
		f.Close()
		if err := exec.Command("open", "-a", app, name).Run(); err != nil {
			fmt.Fprintln(os.Stderr, "govi:", err)
			return 1
		}
		return 0
	}

	// Resolve to absolute paths, creating any that do not exist (like vi, you
	// can open a new file).
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
	if len(paths) == 0 {
		return 1
	}

	// Record absolute paths for a cold launch: GoVi.app reads launch-files when
	// macOS has not yet delivered the open-documents Apple Event (or when the
	// parent process is not GUI-attached). The app deletes this after consuming
	// it; a running instance gets paths via the normal open event instead.
	if err := writeLines(filepath.Join(supportDir, "launch-files"), paths); err != nil {
		fmt.Fprintln(os.Stderr, "govi:", err)
		return 1
	}

	var fifo string
	if wait {
		fifo, err = makeWaitFifo(supportDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "govi:", err)
			return 1
		}
	}

	// `open -a` launches the app or, if already running, delivers the files to
	// that instance and brings it to the front.
	openArgs := append([]string{"-a", app}, paths...)
	if err := exec.Command("open", openArgs...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "govi:", err)
		return 1
	}

	if wait {
		waitForFifo(fifo)
		os.Remove(fifo)
		os.Remove(filepath.Join(supportDir, "launch-wait"))
	}
	return 0
}

// findGoviApp locates GoVi.app via, in order: $GOVI_APP, alongside the running
// binary (installed layout), the dev build dirs, then /Applications.
func findGoviApp() (string, error) {
	if app := os.Getenv("GOVI_APP"); app != "" {
		if isDir(app) {
			return app, nil
		}
		return "", fmt.Errorf("GOVI_APP=%s is not a directory", app)
	}
	exe, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
	}
	dir := filepath.Dir(exe)
	for _, cand := range []string{
		filepath.Join(dir, "GoVi.app"),                 // installed: ~/bin/GoVi.app
		filepath.Join(dir, "gui", "build", "GoVi.app"), // dev: repo-root binary
		filepath.Join(dir, "build", "GoVi.app"),
		"/Applications/GoVi.app",
	} {
		if isDir(cand) {
			return cand, nil
		}
	}
	return "", fmt.Errorf("cannot find GoVi.app; set GOVI_APP to its path")
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// goviSupportDir returns (creating if needed) the per-user directory GoVi.app
// reads its launch context from.
func goviSupportDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Application Support", "GoVi")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// writeLaunchContext records startup information for GoVi.app: cwd, silent flag,
// the resolved system/home exrc paths, and (when set) the EXINIT/NEXINIT text.
func writeLaunchContext(dir string, silent bool) error {
	// Reset the env-init capture files; rewritten below when set.
	if err := os.WriteFile(filepath.Join(dir, "nexinit"), nil, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "exinit"), nil, 0o644); err != nil {
		return err
	}

	var b strings.Builder
	cwd, _ := os.Getwd()
	fmt.Fprintf(&b, "cwd=%s\n", cwd)
	silentN := 0
	if silent {
		silentN = 1
	}
	fmt.Fprintf(&b, "silent=%d\n", silentN)
	if sys := findSysExrc(); sys != "" {
		fmt.Fprintf(&b, "sys_exrc=%s\n", sys)
	}

	if !silent {
		switch {
		case os.Getenv("NEXINIT") != "":
			if err := os.WriteFile(filepath.Join(dir, "nexinit"), []byte(os.Getenv("NEXINIT")), 0o644); err != nil {
				return err
			}
			b.WriteString("has_nexinit=1\n")
		case os.Getenv("EXINIT") != "":
			if err := os.WriteFile(filepath.Join(dir, "exinit"), []byte(os.Getenv("EXINIT")), 0o644); err != nil {
				return err
			}
			b.WriteString("has_exinit=1\n")
		default:
			if home := findHomeExrc(); home != "" {
				fmt.Fprintf(&b, "home_exrc=%s\n", home)
			}
		}
	}

	return os.WriteFile(filepath.Join(dir, "launch-context"), []byte(b.String()), 0o644)
}

func findSysExrc() string {
	if exrcAllowed("/etc/vi.exrc", true) {
		return "/etc/vi.exrc"
	}
	return ""
}

func findHomeExrc() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if p := filepath.Join(home, ".nexrc"); exrcAllowed(p, false) {
		return p
	}
	if p := filepath.Join(home, ".exrc"); exrcAllowed(p, false) {
		return p
	}
	return ""
}

// exrcAllowed reports whether path may be sourced, mirroring nvi exrc_isok: the
// file must exist, must not be group- or world-writable, and must be owned by
// the user (or the user must be root). rootOwn additionally lets a root-owned
// file pass, as for /etc/vi.exrc.
func exrcAllowed(path string, rootOwn bool) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	uid := int(st.Uid)
	euid := os.Geteuid()
	if rootOwn {
		return uid == 0 || euid == 0 || uid == euid
	}
	return euid == 0 || uid == euid
}

func writeLines(path string, lines []string) error {
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// makeWaitFifo creates a FIFO for -w and records its path in launch-wait for
// GoVi.app to signal when the opened tabs/windows have closed.
func makeWaitFifo(supportDir string) (string, error) {
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
	if err := os.WriteFile(filepath.Join(supportDir, "launch-wait"), []byte("fifo="+path+"\n"), 0o644); err != nil {
		os.Remove(path)
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
