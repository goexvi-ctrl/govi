package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// colonWordStart returns the rune index where the blank-delimited word before
// the cursor begins (nvi txt_fc).
func colonWordStart(colon []rune) int {
	i := len(colon)
	for i > 0 && colon[i-1] != ' ' && colon[i-1] != '\t' {
		i--
	}
	return i
}

func (e *Engine) colonFilecKey(ev KeyEvent) bool {
	fc := e.scr.opts.Str("filec")
	if fc == "" {
		return false
	}
	ch := rune(fc[0])
	if ev.Rune == ch {
		return true
	}
	return ch == '\t' && ev.Key == KeyTab
}

// colonDoFileComplete performs file name completion on the colon command line
// (nvi txt_fc / argv_lexp).
func (e *Engine) colonDoFileComplete() {
	e.ensureCwd()
	trydir := false
	for {
		start := colonWordStart(e.scr.colon)
		word := string(e.scr.colon[start:])
		matches, err := globFileNames(word, e.cwd)
		if err != nil {
			e.fe.Bell()
			return
		}
		switch len(matches) {
		case 0:
			if !trydir {
				e.fe.Bell()
			}
			return
		case 1:
			repl := matches[0]
			if repl == word {
				full := e.resolvePath(strings.TrimSuffix(repl, "/"))
				if info, err := os.Stat(full); err == nil && info.IsDir() {
					if !trydir {
						e.replaceColonWord(start, strings.TrimSuffix(repl, "/")+"/")
						trydir = true
						continue
					}
					e.fe.Bell()
					return
				}
				if !trydir {
					e.fe.Bell()
				}
				return
			}
			e.replaceColonWord(start, repl)
			full := e.resolvePath(strings.TrimSuffix(repl, "/"))
			if info, err := os.Stat(full); err == nil && info.IsDir() {
				e.scr.colon = append(e.scr.colon, '/')
				trydir = true
				continue
			}
			e.fe.Render(view{e.scr}, ChangeSet{MessageChanged: true})
			return
		default:
			prefix := commonStringPrefix(matches)
			e.replaceColonWord(start, prefix)
			e.showOutput(formatFileList(matches, e.bangCols()))
			e.fe.Render(view{e.scr}, ChangeSet{Full: true})
			return
		}
	}
}

func (e *Engine) replaceColonWord(start int, repl string) {
	e.scr.colon = append(append([]rune(nil), e.scr.colon[:start]...), []rune(repl)...)
}

// globFileNames returns file names relative to cwd matching word as a prefix
// glob (word plus implicit *).
func globFileNames(word, cwd string) ([]string, error) {
	word = expandPathTilde(word, cwd)
	dirPart, prefix := splitFilePattern(word)
	searchDir := cwd
	if dirPart != "" {
		if filepath.IsAbs(dirPart) {
			searchDir = filepath.Clean(dirPart)
		} else {
			searchDir = filepath.Join(cwd, dirPart)
		}
	}
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, ent := range entries {
		name := ent.Name()
		if prefix == "" && strings.HasPrefix(name, ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		match := name
		if dirPart != "" {
			match = joinDisplayPath(dirPart, name)
		}
		matches = append(matches, match)
	}
	sort.Strings(matches)
	return matches, nil
}

func splitFilePattern(word string) (dir, prefix string) {
	if word == "" {
		return "", ""
	}
	if i := strings.LastIndex(word, "/"); i >= 0 {
		return word[:i+1], word[i+1:]
	}
	return "", word
}

func joinDisplayPath(dir, name string) string {
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func expandPathTilde(word, cwd string) string {
	if word == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return word
	}
	if strings.HasPrefix(word, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, word[2:])
		}
	}
	return word
}

func commonStringPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	p := strs[0]
	for _, s := range strs[1:] {
		for len(p) > 0 && (len(s) < len(p) || s[:len(p)] != p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

func commonDirPrefix(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	p := paths[0]
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[:i+1]
	} else {
		return ""
	}
	for _, s := range paths[1:] {
		j := strings.LastIndex(s, "/")
		if j < 0 {
			return ""
		}
		dir := s[:j+1]
		for len(p) > 0 && (len(dir) < len(p) || dir[:len(p)] != p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

// formatFileList builds a multi-column file listing for the completion overlay
// (nvi txt_fc_col).
func formatFileList(paths []string, cols int) []string {
	if cols < 1 {
		cols = 80
	}
	prefix := commonDirPrefix(paths)
	names := make([]string, len(paths))
	max := 0
	for i, p := range paths {
		names[i] = strings.TrimPrefix(p, prefix)
		if len(names[i]) > max {
			max = len(names[i])
		}
	}
	colWidth := max
	if colWidth%6 != 0 {
		colWidth += 6 - colWidth%6
	}
	if colWidth < 6 {
		colWidth = 6
	}
	if colWidth > cols {
		lines := make([]string, len(names))
		for i, n := range names {
			lines[i] = n
		}
		return lines
	}
	numCols := (cols - 1) / colWidth
	if numCols < 1 {
		numCols = 1
	}
	numRows := (len(names) + numCols - 1) / numCols
	var lines []string
	for row := 0; row < numRows; row++ {
		var b strings.Builder
		for col := 0; col < numCols; col++ {
			idx := row + col*numRows
			if idx >= len(names) {
				break
			}
			s := names[idx]
			b.WriteString(s)
			if col+1 < numCols && row+col*numRows+numRows < len(names) {
				if pad := colWidth - len(s); pad > 0 {
					b.WriteString(strings.Repeat(" ", pad))
				}
			}
		}
		lines = append(lines, b.String())
	}
	return lines
}