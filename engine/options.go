package engine

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Options are stored generically by name so the full nvi option set is easy to
// carry. Only some options drive implemented behavior; the rest are recognized
// and settable (and listed by :set all) but inert. This mirrors nvi's O_*
// table (common/options.c, options_def.h).

type optType int

const (
	optBool optType = iota
	optNum
	optStr
)

type optDef struct {
	name string
	abbr string // short form, or ""
	typ  optType
	dB   bool
	dN   int
	dS   string
}

// optDefs is the option table. Defaults follow nvi's. A boolean shown without
// "no" in nvi's :set all is default-on (dB true).
var optDefs = []optDef{
	{name: "altwerase", typ: optBool},
	{name: "autoindent", abbr: "ai", typ: optBool},
	{name: "autoprint", abbr: "ap", typ: optBool, dB: true},
	{name: "autowrite", abbr: "aw", typ: optBool},
	{name: "backup", typ: optStr},
	{name: "beautify", abbr: "bf", typ: optBool},
	{name: "cdpath", typ: optStr},
	{name: "cedit", typ: optStr},
	{name: "columns", abbr: "co", typ: optNum, dN: 80},
	{name: "comment", typ: optBool},
	{name: "directory", abbr: "dir", typ: optStr, dS: "/tmp"},
	{name: "edcompatible", abbr: "ed", typ: optBool},
	{name: "errorbells", abbr: "eb", typ: optBool},
	{name: "escapetime", typ: optNum, dN: 1},
	{name: "exrc", typ: optBool},
	{name: "extended", typ: optBool},
	{name: "filec", typ: optStr},
	{name: "flash", typ: optBool, dB: true},
	{name: "hardtabs", abbr: "ht", typ: optNum},
	{name: "iclower", typ: optBool},
	{name: "ignorecase", abbr: "ic", typ: optBool},
	{name: "keytime", typ: optNum, dN: 6},
	{name: "leftright", typ: optBool},
	{name: "lines", typ: optNum, dN: 24},
	{name: "lisp", typ: optBool},
	{name: "list", typ: optBool},
	{name: "lock", typ: optBool, dB: true},
	{name: "magic", typ: optBool, dB: true},
	{name: "matchtime", typ: optNum, dN: 7},
	{name: "mesg", typ: optBool, dB: true},
	{name: "modeline", typ: optBool},
	{name: "number", abbr: "nu", typ: optBool},
	{name: "octal", typ: optBool},
	{name: "open", typ: optBool, dB: true},
	{name: "optimize", abbr: "opt", typ: optBool, dB: true},
	{name: "paragraphs", abbr: "para", typ: optStr, dS: "IPLPPPQPP LIpplpipbp"},
	{name: "path", typ: optStr},
	{name: "print", typ: optStr},
	{name: "prompt", typ: optBool, dB: true},
	{name: "readonly", abbr: "ro", typ: optBool},
	{name: "redraw", abbr: "re", typ: optBool},
	{name: "remap", typ: optBool, dB: true},
	{name: "report", typ: optNum, dN: 5},
	{name: "ruler", typ: optBool},
	{name: "scroll", abbr: "scr", typ: optNum},
	{name: "searchincr", typ: optBool},
	{name: "secure", typ: optBool},
	{name: "sections", abbr: "sect", typ: optStr, dS: "NHSHH HUnhsh"},
	{name: "shell", abbr: "sh", typ: optStr, dS: "/bin/sh"},
	{name: "shellmeta", typ: optStr, dS: "~{[*?$`'\"\\"},
	{name: "shiftwidth", abbr: "sw", typ: optNum, dN: 8},
	{name: "showmatch", abbr: "sm", typ: optBool},
	{name: "showmode", abbr: "smd", typ: optBool},
	{name: "sidescroll", typ: optNum, dN: 16},
	{name: "slowopen", abbr: "slow", typ: optBool},
	{name: "sourceany", typ: optBool},
	{name: "tabstop", abbr: "ts", typ: optNum, dN: 8},
	{name: "taglength", abbr: "tl", typ: optNum},
	{name: "tags", typ: optStr, dS: "tags"},
	{name: "term", typ: optStr},
	{name: "terse", typ: optBool},
	{name: "tildeop", abbr: "to", typ: optBool},
	{name: "timeout", typ: optBool, dB: true},
	{name: "ttywerase", typ: optBool},
	{name: "verbose", typ: optBool},
	{name: "warn", typ: optBool, dB: true},
	{name: "window", abbr: "w", typ: optNum},
	{name: "windowname", typ: optBool},
	{name: "wraplen", abbr: "wl", typ: optNum},
	{name: "wrapmargin", abbr: "wm", typ: optNum},
	{name: "wrapscan", abbr: "ws", typ: optBool, dB: true},
	{name: "writeany", abbr: "wa", typ: optBool},
}

// optByName resolves a name or abbreviation to its definition.
var optByName = func() map[string]*optDef {
	m := make(map[string]*optDef, len(optDefs)*2)
	for i := range optDefs {
		d := &optDefs[i]
		m[d.name] = d
		if d.abbr != "" {
			m[d.abbr] = d
		}
	}
	return m
}()

// options holds the per-screen option values.
type options struct {
	b map[string]bool
	i map[string]int
	s map[string]string
}

func defaultOptions() options {
	o := options{b: map[string]bool{}, i: map[string]int{}, s: map[string]string{}}
	for i := range optDefs {
		d := &optDefs[i]
		switch d.typ {
		case optBool:
			o.b[d.name] = d.dB
		case optNum:
			o.i[d.name] = d.dN
		case optStr:
			o.s[d.name] = d.dS
		}
	}
	// Environment-derived defaults, like nvi.
	if sh := os.Getenv("SHELL"); sh != "" {
		o.s["shell"] = sh
	}
	if term := os.Getenv("TERM"); term != "" {
		o.s["term"] = term
	}
	return o
}

// Accessors used by the rest of the engine.
func (o options) Bool(name string) bool  { return o.b[name] }
func (o options) Int(name string) int    { return o.i[name] }
func (o options) Str(name string) string { return o.s[name] }

// exSet implements :set.
func (e *Engine) exSet(c *exCmd) error {
	arg := strings.TrimSpace(c.arg)
	if arg == "" {
		e.showOptions(false)
		return nil
	}
	if arg == "all" {
		e.showOptions(true)
		return nil
	}
	for _, tok := range strings.Fields(arg) {
		if err := e.setOne(tok); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) setOne(tok string) error {
	o := &e.scr.opts

	// name=value
	if i := strings.IndexByte(tok, '='); i >= 0 {
		name, val := tok[:i], tok[i+1:]
		d, ok := optByName[name]
		if !ok {
			return fmt.Errorf("set: no %s option", name)
		}
		switch d.typ {
		case optStr:
			o.s[d.name] = val
		case optNum:
			n, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("set: %s: illegal value %q", d.name, val)
			}
			o.i[d.name] = n
		default:
			return fmt.Errorf("set: %s is not a settable-value option", d.name)
		}
		return nil
	}

	// name? (query)
	if strings.HasSuffix(tok, "?") {
		d, ok := optByName[tok[:len(tok)-1]]
		if !ok {
			return fmt.Errorf("set: no %s option", tok[:len(tok)-1])
		}
		e.scr.msg, e.scr.msgKind = e.optDisplay(d), MsgInfo
		return nil
	}

	// name!  (toggle a boolean)
	toggle := false
	if strings.HasSuffix(tok, "!") {
		toggle = true
		tok = tok[:len(tok)-1]
	}

	// noname  (clear a boolean)
	val := true
	name := tok
	if _, known := optByName[tok]; !known && strings.HasPrefix(tok, "no") {
		val = false
		name = tok[2:]
	}
	d, ok := optByName[name]
	if !ok {
		return fmt.Errorf("set: no %s option", name)
	}
	if d.typ != optBool {
		return fmt.Errorf("set: %s is not a boolean option", d.name)
	}
	if toggle {
		o.b[d.name] = !o.b[d.name]
	} else {
		o.b[d.name] = val
	}
	return nil
}

// optDisplay formats one option as nvi does: "name"/"noname" for booleans,
// "name=value" for numerics and strings (strings quoted).
func (e *Engine) optDisplay(d *optDef) string {
	o := &e.scr.opts
	switch d.typ {
	case optBool:
		if o.b[d.name] {
			return d.name
		}
		return "no" + d.name
	case optNum:
		return fmt.Sprintf("%s=%d", d.name, o.i[d.name])
	default:
		return fmt.Sprintf("%s=%q", d.name, o.s[d.name])
	}
}

// isDefault reports whether option d currently holds its default value.
func (e *Engine) isDefault(d *optDef) bool {
	o := &e.scr.opts
	switch d.typ {
	case optBool:
		return o.b[d.name] == d.dB
	case optNum:
		return o.i[d.name] == d.dN
	default:
		return o.s[d.name] == d.dS
	}
}

// showOptions renders the option list (all, or only those changed from their
// defaults) as a multi-column grid and shows it. Long values get their own
// full-width line, matching nvi's :set output layout.
func (e *Engine) showOptions(all bool) {
	width := e.scr.cols
	if width < 1 {
		width = 80
	}

	// Collect the options to show, sorted by option NAME. The "no" prefix on a
	// disabled boolean is only a display modifier -- nvi sorts on the bare name,
	// so "noruler" sorts as "ruler" (after "open"), not under "n".
	type shown struct {
		name string
		disp string
	}
	var opts []shown
	longBool := 1
	for i := range optDefs {
		d := &optDefs[i]
		if !all && e.isDefault(d) {
			continue
		}
		disp := e.optDisplay(d)
		opts = append(opts, shown{name: d.name, disp: disp})
		if d.typ == optBool && len(disp) > longBool {
			longBool = len(disp)
		}
	}
	if len(opts) == 0 {
		return
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].name < opts[j].name })

	colW := longBool + 2
	var short, long []string
	for _, o := range opts {
		if len(o.disp) <= longBool {
			short = append(short, o.disp)
		} else {
			long = append(long, o.disp)
		}
	}

	cols := width / colW
	if cols < 1 {
		cols = 1
	}
	rows := (len(short) + cols - 1) / cols

	var out []string
	for r := 0; r < rows; r++ {
		var b strings.Builder
		for c := 0; c < cols; c++ {
			idx := c*rows + r
			if idx >= len(short) {
				continue
			}
			s := short[idx]
			b.WriteString(s)
			// Pad, except after the last column of the row.
			if c < cols-1 && c*rows+rows+r < len(short)+rows {
				for k := len(s); k < colW; k++ {
					b.WriteByte(' ')
				}
			}
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	out = append(out, long...)

	e.showOutput(out)
}
