package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// makeBackup copies the current on-disk contents of the file being written to a
// backup file before the write overwrites it, when the backup option is set
// (nvi common/exf.c file_backup). The option is a name pattern: '%' expands to
// the current file name, '%%' is a literal '%', and a leading 'N' requests a
// versioned backup whose name ends in an incrementing number. The backup is
// written 0600 and refused if a pre-existing backup is not a private regular
// file. It is a no-op when backup is unset or the target does not exist yet.
func (e *Engine) makeBackup(target string) error {
	pat := strings.TrimSpace(e.scr.opts.Str("backup"))
	if pat == "" {
		return nil
	}
	src, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing on disk to back up
		}
		return err
	}
	versioned := strings.HasPrefix(pat, "N")
	if versioned {
		pat = pat[1:]
	}
	name := expandBackupPercent(pat, e.scr.name)
	if name == "" {
		return fmt.Errorf("backup: empty backup file name")
	}
	bname := e.resolvePath(name)
	if versioned {
		bname = nextBackupVersion(bname)
	}
	// Refuse a pre-existing backup that is not a private regular file (nvi's
	// "not a regular file" / "accessible by a user other than the owner" checks).
	if info, err := os.Stat(bname); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: not a regular file", name)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("%s: accessible by a user other than the owner", name)
		}
	}
	return os.WriteFile(bname, src, 0o600)
}

// expandBackupPercent replaces each unescaped '%' in the backup pattern with the
// current file name, and '%%' with a literal '%' (nvi argv_exp2's file-name
// expansion, restricted to the '%' current-file token).
func expandBackupPercent(pat, curName string) string {
	var b strings.Builder
	for i := 0; i < len(pat); i++ {
		if pat[i] != '%' {
			b.WriteByte(pat[i])
			continue
		}
		if i+1 < len(pat) && pat[i+1] == '%' {
			b.WriteByte('%')
			i++
		} else {
			b.WriteString(curName)
		}
	}
	return b.String()
}

// nextBackupVersion returns base with the next unused integer suffix appended,
// scanning base's directory for existing base<number> files (nvi's leading-'N'
// versioned backups).
func nextBackupVersion(base string) string {
	dir, prefix := filepath.Dir(base), filepath.Base(base)
	max := 0
	if entries, err := os.ReadDir(dir); err == nil {
		for _, ent := range entries {
			if n := ent.Name(); strings.HasPrefix(n, prefix) {
				if v, err := strconv.Atoi(n[len(prefix):]); err == nil && v > max {
					max = v
				}
			}
		}
	}
	return base + strconv.Itoa(max+1)
}
