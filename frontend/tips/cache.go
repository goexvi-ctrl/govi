package tips

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cache lazily loads and re-loads one tooltip file. Get re-stats the file on
// every call (it runs at hover-timer frequency, not per mouse move) so edits
// to the file show up without restarting the editor; the parse is redone only
// when the path, modification time, or size changed. A missing or unreadable
// file yields a nil Table, i.e. no tooltips.
type Cache struct {
	path  string // expanded path last examined
	mtime time.Time
	size  int64
	ok    bool
	tbl   Table
}

// Get returns the Table for path (a leading "~/" expands to the home
// directory; empty means no tooltip file).
func (c *Cache) Get(path string) Table {
	if path == "" {
		c.ok = false
		return nil
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		c.path, c.tbl, c.ok = path, nil, false
		return nil
	}
	if c.ok && path == c.path && st.ModTime().Equal(c.mtime) && st.Size() == c.size {
		return c.tbl
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.path, c.tbl, c.ok = path, nil, false
		return nil
	}
	c.path, c.mtime, c.size = path, st.ModTime(), st.Size()
	c.tbl, c.ok = Parse(string(data)), true
	return c.tbl
}
