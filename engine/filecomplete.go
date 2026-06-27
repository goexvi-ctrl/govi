package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
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

// exFilePathCmds lists ex commands whose argument is a file or directory path.
var exFilePathCmds = map[string]bool{
	"edit":    true,
	"write":   true,
	"read":    true,
	"file":    true,
	"source":  true,
	"recover": true,
	"cd":      true,
	"chdir":   true,
}

// colonExpectsPathArg reports whether the blank-delimited word before the end
// of the colon line is a file-path argument (e.g. ":w path", ":r path").
func colonExpectsPathArg(colon []rune) bool {
	p := &exParser{s: colon}
	p.skipBlanks()
	if p.eof() {
		return false
	}
	cur := int64(1)
	if p.peek() == '%' {
		p.next()
	} else if _, ok := p.parseAddr(cur); ok {
		p.skipBlanks()
		for p.peek() == ',' || p.peek() == ';' {
			p.next()
			p.skipBlanks()
			if a2, ok := p.parseAddr(cur); ok {
				cur = a2
			}
			p.skipBlanks()
		}
	}
	p.skipBlanks()
	cmdStart := p.pos
	name := p.parseName()
	if name == "" {
		return false
	}
	def, err := findCmd(name)
	if err != nil || !exFilePathCmds[def.full] {
		return false
	}
	if p.peek() == '!' {
		p.next()
	}
	p.skipBlanks()
	argStart := p.pos
	wordStart := colonWordStart(colon)
	return wordStart >= argStart && wordStart > cmdStart
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
			// Ambiguous: stay on the colon line; user types more to narrow it.
			e.fe.Bell()
			return
		}
	}
}

func (e *Engine) replaceColonWord(start int, repl string) {
	merged := append(append([]rune(nil), e.scr.colon[:start]...), []rune(repl)...)
	e.scr.colon = []rune(norm.NFC.String(string(merged)))
}

// globFileNames returns file names relative to cwd matching word as a prefix
// glob (word plus implicit *).
func globFileNames(word, cwd string) ([]string, error) {
	word = expandPathTilde(word, cwd)
	dirPart, prefix := splitFilePattern(word)
	searchDir, displayDir := fileCompletionSearchDir(dirPart, cwd)
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
		if displayDir != "" {
			match = joinDisplayPath(displayDir, name)
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

// fileCompletionSearchDir maps the directory portion of a colon-line path to the
// directory to read and the prefix to show when building match strings.
func fileCompletionSearchDir(dirPart, cwd string) (searchDir, displayDir string) {
	if dirPart == "" {
		return cwd, ""
	}
	if strings.HasPrefix(dirPart, "/") {
		searchDir = filepath.Clean(dirPart)
		if searchDir == "." {
			searchDir = "/"
		}
		return searchDir, searchDir
	}
	searchDir = filepath.Join(cwd, dirPart)
	return searchDir, dirPart
}

func joinDisplayPath(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
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
