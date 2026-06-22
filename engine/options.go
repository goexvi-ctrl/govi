package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// options holds the editor settings controlled by :set, corresponding to nvi's
// O_* options (common/options.c). Only the options that affect implemented
// behavior are present; more can be added as features land.
type options struct {
	autoindent bool
	ignorecase bool
	magic      bool
	wrapscan   bool
	number     bool
	list       bool
	tabstop    int
	shiftwidth int
}

func defaultOptions() options {
	return options{
		magic:      true,
		wrapscan:   true,
		tabstop:    8,
		shiftwidth: 8,
	}
}

// optAbbrev maps option names and their standard abbreviations to a canonical
// name.
var optAbbrev = map[string]string{
	"autoindent": "autoindent", "ai": "autoindent",
	"ignorecase": "ignorecase", "ic": "ignorecase",
	"magic":    "magic",
	"wrapscan": "wrapscan", "ws": "wrapscan",
	"number": "number", "nu": "number",
	"list":       "list",
	"tabstop":    "tabstop", "ts": "tabstop",
	"shiftwidth": "shiftwidth", "sw": "shiftwidth",
}

func optIsBool(canon string) bool {
	switch canon {
	case "tabstop", "shiftwidth":
		return false
	}
	return true
}

// exSet implements :set.
func (e *Engine) exSet(c *exCmd) error {
	arg := strings.TrimSpace(c.arg)
	if arg == "" || arg == "all" {
		e.scr.msg, e.scr.msgKind = e.optSummary(), MsgInfo
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
		canon, ok := optAbbrev[name]
		if !ok {
			return fmt.Errorf("set: no %s option", name)
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("set: %s: illegal value %q", canon, val)
		}
		switch canon {
		case "tabstop":
			if n < 1 {
				n = 1
			}
			o.tabstop = n
		case "shiftwidth":
			if n < 1 {
				n = 1
			}
			o.shiftwidth = n
		default:
			return fmt.Errorf("set: %s is not a numeric option", canon)
		}
		return nil
	}

	// name?  (query)
	if strings.HasSuffix(tok, "?") {
		canon, ok := optAbbrev[tok[:len(tok)-1]]
		if !ok {
			return fmt.Errorf("set: no %s option", tok[:len(tok)-1])
		}
		e.scr.msg, e.scr.msgKind = e.optDisplay(canon), MsgInfo
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
	if strings.HasPrefix(tok, "no") {
		if _, ok := optAbbrev[tok]; !ok { // "number" starts with "no"? no; safe
			val = false
			name = tok[2:]
		}
	}
	canon, ok := optAbbrev[name]
	if !ok {
		return fmt.Errorf("set: no %s option", name)
	}
	if !optIsBool(canon) {
		return fmt.Errorf("set: %s is not a boolean option", canon)
	}
	cur := e.optBool(canon)
	switch {
	case toggle:
		e.setBool(canon, !cur)
	default:
		e.setBool(canon, val)
	}
	return nil
}

func (e *Engine) optBool(canon string) bool {
	o := &e.scr.opts
	switch canon {
	case "autoindent":
		return o.autoindent
	case "ignorecase":
		return o.ignorecase
	case "magic":
		return o.magic
	case "wrapscan":
		return o.wrapscan
	case "number":
		return o.number
	case "list":
		return o.list
	}
	return false
}

func (e *Engine) setBool(canon string, v bool) {
	o := &e.scr.opts
	switch canon {
	case "autoindent":
		o.autoindent = v
	case "ignorecase":
		o.ignorecase = v
	case "magic":
		o.magic = v
	case "wrapscan":
		o.wrapscan = v
	case "number":
		o.number = v
	case "list":
		o.list = v
	}
}

func (e *Engine) optDisplay(canon string) string {
	if optIsBool(canon) {
		if e.optBool(canon) {
			return canon
		}
		return "no" + canon
	}
	o := &e.scr.opts
	switch canon {
	case "tabstop":
		return fmt.Sprintf("tabstop=%d", o.tabstop)
	case "shiftwidth":
		return fmt.Sprintf("shiftwidth=%d", o.shiftwidth)
	}
	return canon
}

func (e *Engine) optSummary() string {
	canon := map[string]bool{}
	for _, c := range optAbbrev {
		canon[c] = true
	}
	names := make([]string, 0, len(canon))
	for c := range canon {
		names = append(names, c)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, c := range names {
		parts[i] = e.optDisplay(c)
	}
	return strings.Join(parts, " ")
}
