//go:build unix

package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	pathSysExrc = "/etc/vi.exrc"
	pathNexrc   = ".nexrc"
	pathExrc    = ".exrc"
)

type exrcVerdict int

const (
	exrcNoExist exrcVerdict = iota
	exrcNoPerm
	exrcOK
)

type fileID struct {
	dev uint64
	ino uint64
	ok  bool
}

func statID(info os.FileInfo) fileID {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}
	}
	return fileID{dev: uint64(st.Dev), ino: st.Ino, ok: true}
}

// LoadStartup reads nvi startup information before the file to edit is opened.
// Order: /etc/vi.exrc; NEXINIT or EXINIT; $HOME/.nexrc or .exrc; ./.nexrc or
// .exrc when the exrc option is set. See nvi "Startup Information" (ex_init.c).
func (e *Engine) LoadStartup() error {
	ctx := e.launchCtx
	if ctx.Silent {
		return nil
	}

	var homeID fileID

	sysPath := pathSysExrc
	if ctx.SysExrc != "" {
		sysPath = ctx.SysExrc
	}
	if _, _, err := e.tryStartupFile(sysPath, true, false); err != nil {
		return err
	}

	nexinit := os.Getenv("NEXINIT")
	if ctx.Nexinit != "" {
		nexinit = ctx.Nexinit
	}
	exinit := os.Getenv("EXINIT")
	if ctx.Exinit != "" {
		exinit = ctx.Exinit
	}

	if nexinit != "" {
		if err := e.runStartupScript("NEXINIT", nexinit); err != nil {
			return err
		}
	} else if exinit != "" {
		if err := e.runStartupScript("EXINIT", exinit); err != nil {
			return err
		}
	} else if ctx.HomeExrc != "" {
		v, info, err := e.tryStartupFile(ctx.HomeExrc, false, true)
		if err != nil {
			return err
		}
		if v == exrcOK {
			homeID = statID(info)
		}
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		nex := filepath.Join(home, pathNexrc)
		v, info, err := e.tryStartupFile(nex, false, true)
		if err != nil {
			return err
		}
		switch v {
		case exrcOK:
			homeID = statID(info)
		case exrcNoExist:
			ex := filepath.Join(home, pathExrc)
			v, info, err = e.tryStartupFile(ex, false, true)
			if err != nil {
				return err
			}
			if v == exrcOK {
				homeID = statID(info)
			}
		}
	}

	if e.quit {
		return nil
	}

	if !e.scr.opts.Bool("exrc") {
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	if ctx.Cwd != "" {
		cwd = ctx.Cwd
	}

	v, info, err := e.tryStartupFile(filepath.Join(cwd, pathNexrc), false, false)
	if err != nil {
		return err
	}
	switch v {
	case exrcNoExist:
		v, info, err = e.tryStartupFile(filepath.Join(cwd, pathExrc), false, false)
		if err != nil {
			return err
		}
		if v == exrcOK && sameStartupFile(homeID, info) {
			return nil
		}
	case exrcOK:
		if sameStartupFile(homeID, info) {
			return nil
		}
	}
	return nil
}

func sameStartupFile(home fileID, local os.FileInfo) bool {
	if !home.ok {
		return false
	}
	id := statID(local)
	return id.ok && id.dev == home.dev && id.ino == home.ino
}

func (e *Engine) tryStartupFile(path string, rootOwn, rootID bool) (exrcVerdict, os.FileInfo, error) {
	v, info, err := exrcAllowed(path, rootOwn, rootID)
	if err != nil {
		return exrcNoPerm, nil, err
	}
	if v != exrcOK {
		return v, info, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return exrcNoPerm, info, fmt.Errorf("%s: %w", path, err)
	}
	if len(data) > maxExrcSize {
		return exrcNoPerm, info, fmt.Errorf("%s: file too large", path)
	}
	if err := e.runStartupScript(path, string(data)); err != nil {
		return exrcNoPerm, info, err
	}
	return exrcOK, info, nil
}

func exrcAllowed(path string, rootOwn, rootID bool) (exrcVerdict, os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return exrcNoExist, nil, nil
		}
		return exrcNoPerm, nil, err
	}
	euid := os.Geteuid()
	uid := fileUID(info)
	allowed := (rootOwn && uid == 0) || (rootID && euid == 0) || int(uid) == euid
	if !allowed {
		exrcDeny(path, rootOwn)
		return exrcNoPerm, info, nil
	}
	if info.Mode().Perm()&0022 != 0 {
		fmt.Fprintf(os.Stderr, "nvi: %s: not sourced: writeable by a user other than the owner\n", path)
		return exrcNoPerm, info, nil
	}
	return exrcOK, info, nil
}

func fileUID(info os.FileInfo) uint32 {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ^uint32(0)
	}
	return st.Uid
}

func exrcDeny(path string, rootOwn bool) {
	if rootOwn {
		fmt.Fprintf(os.Stderr, "nvi: %s: not sourced: not owned by you or root\n", path)
	} else {
		fmt.Fprintf(os.Stderr, "nvi: %s: not sourced: not owned by you\n", path)
	}
}
