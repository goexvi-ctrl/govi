package engine

import (
	"fmt"
	"sort"
	"strings"
)

// exCmdUsage holds :exusage text for an implemented ex command. Update this
// table when adding or changing ex commands (TestExUsageCoverage enforces it).
type exCmdUsage struct {
	summary string
	usage   string
}

// exCmdMeta maps canonical ex command names to usage strings.
var exCmdMeta = map[string]exCmdUsage{
	"!":            {"filter lines through a shell command", ":[range] !cmd\n:!cmd"},
	"&":            {"repeat the last substitute", ":&"},
	"<":            {"shift lines left", ":[range] < [count]"},
	">":            {"shift lines right", ":[range] > [count]"},
	"=":            {"display line number", ":[line]="},
	"abbreviate":   {"define an input abbreviation", ":ab[breviate] lhs rhs"},
	"append":       {"append text after line", ":[line] append"},
	"args":         {"display the argument list", ":ar[gs]"},
	"cd":           {"change working directory", ":cd[!] [directory]"},
	"chdir":        {"change working directory", ":chd[ir][!] [directory]"},
	"change":       {"change lines", ":[range] change"},
	"copy":         {"copy lines", ":[range] copy [buffer] address"},
	"delete":       {"delete lines", ":[range] delete [buffer]"},
	"edit":         {"edit a file", ":e[dit][!] [file]"},
	"exusage":      {"display ex command usage", ":exu[sage] [command]"},
	"file":         {"display or set the file name", ":f[ile] [name]"},
	"global":       {"execute commands on matching lines", ":[range] g[lobal] /pattern/ commands"},
	"help":         {"display help statement", ":he[lp]"},
	"insert":       {"insert text before line", ":[line] insert"},
	"join":         {"join lines", ":[range] join"},
	"list":         {"print lines with visible characters", ":[range] list"},
	"map":          {"map keys in command mode", ":map[!] lhs rhs"},
	"move":         {"move lines", ":[range] move address"},
	"Next":         {"edit previous file in the argument list", ":Next[!]"},
	"next":         {"edit next file in the argument list", ":n[ext][!]"},
	"number":       {"print lines with line numbers", ":[range] number"},
	"preserve":     {"preserve file in recovery directory", ":pre[serve]"},
	"previous":     {"edit previous file in the argument list", ":prev[ious][!]"},
	"print":        {"print lines", ":[range] print"},
	"put":          {"put buffer after line", ":[line] pu[t] [buffer]"},
	"quit":         {"quit", ":q[uit][!]"},
	"read":         {"read file after line", ":[line] read file"},
	"recover":      {"recover file from recovery directory", ":rec[over] [file]"},
	"rewind":       {"edit first file in the argument list", ":rew[ind][!]"},
	"set":          {"show or set options", ":se[t] [option[=value]] ..."},
	"shell":        {"run an interactive shell", ":sh[ell]"},
	"source":       {"read and execute ex commands from a file", ":so[urce] file"},
	"stop":         {"suspend the edit session", ":st[op][!]"},
	"substitute":   {"substitute a pattern", ":[range] s/pattern/repl/[flags]"},
	"suspend":      {"suspend the edit session", ":su[spend][!]"},
	"tag":          {"switch to tagged file", ":ta[g] tagname"},
	"t":            {"copy lines", ":[range] t address"},
	"unabbreviate": {"remove an abbreviation", ":una[bbreviate] lhs"},
	"unmap":        {"unmap keys", ":unm[ap][!] lhs"},
	"version":      {"display the editor version", ":ve[rsion]"},
	"vglobal":      {"execute commands on non-matching lines", ":[range] v /pattern/ commands"},
	"visual":       {"enter vi (visual) mode", ":vi[sual]"},
	"viusage":      {"display vi key usage", ":viu[sage] [key]"},
	"write":        {"write the buffer", ":[range] w[rite][!] [file]"},
	"wq":           {"write and quit", ":wq"},
	"xit":          {"write if modified and quit", ":x[it]"},
	"yank":         {"yank lines to a buffer", ":[range] y[ank] [buffer]"},
}

func init() {
	for i := range exCmds {
		if u, ok := exCmdMeta[exCmds[i].full]; ok {
			exCmds[i].summary = u.summary
			exCmds[i].usage = u.usage
		}
	}
}

// viUsageList is the :viusage listing (command and insert mode). Update when
// adding vi features.
var viUsageList = []string{
	"^A\tsearch forward for word under cursor",
	"^B\tpage backward",
	"^D\tscroll down half screen",
	"^E\tscroll down one line",
	"^F\tpage forward",
	"^G\tfile information",
	"^H h\tmove left",
	"^J ^N j\tmove down",
	"^K ^P k\tmove up",
	"^L ^R\trepaint screen",
	"^M +\tnext line, first non-blank",
	"-\tprevious line, first non-blank",
	"^T\ttag pop",
	"^U\tscroll up half screen",
	"^Y\tscroll up one line",
	"^Z\tsuspend editor (terminal)",
	"^]\ttag push",
	"^^\talternate file",
	":\tex command line",
	"/\tsearch forward",
	"?\tsearch backward",
	"n\trepeat last search",
	"N\treverse last search",
	"!\tfilter through shell",
	"#\tincrement/decrement number",
	"$\tend of line",
	"0\tbeginning of line",
	"^\tfirst non-blank on line",
	"|\tcolumn",
	"%\tmatch bracket",
	"&\trepeat substitute",
	"(\t)\tsentence",
	"{ }\tparagraph",
	"[[\tsection backward",
	"]]\tsection forward",
	";\trepeat last f/F/t/T",
	",\treverse last f/F/t/T",
	".\trepeat last change",
	"<\tshift left",
	">\tshift right",
	"@\texecute register",
	"~\ttoggle case",
	"` '\tmarks",
	"a A i I o O\tinsert text",
	"b B w W e E\tword motions",
	"c C d D s S y Y\tchange/delete/yank",
	"f F t T\tfind character in line",
	"G\tgoto line",
	"H M L\tscreen line",
	"J\tjoin lines",
	"m\tset mark",
	"p P\tput",
	"Q\tex mode",
	"r R\treplace",
	"U\trestore line",
	"u\tundo",
	"x X\tdelete character",
	"z\treposition screen",
	"ZZ ZQ\twrite-quit / quit",
	"--- insert mode ---",
	"^@ (NUL)\treplay previous insertion",
	"^D\tshift left",
	"^T\tshift right",
	"^H erase\tdelete previous character",
	"^V\tquote next character",
	"^W\tdelete previous word",
	"^X[hex]\tinsert a Unicode code point (up to 6 hex digits)",
	"^Z\tsuspend (leave insert mode)",
	"<esc>\tend insert",
}

// viUsageDetail maps a single command-mode key to detailed usage text.
var viUsageDetail = map[string]string{
	"!": "!motion cmd\n  filter text through shell command",
	"#": "[count]#+ | [count]#-\n  increment or decrement number at cursor",
	"$": "[count]$\n  move to end of line",
	"%": "%\n  move to matching bracket",
	"&": "&\n  repeat last substitute",
	"'": "'{mark}\n  move to line of mark",
	"(": "[count](\n  move backward [count] sentences",
	")": "[count])\n  move forward [count] sentences",
	",": ",\n  repeat last f/F/t/T in opposite direction",
	"-": "[count]-\n  move to first non-blank of previous line",
	".": "[count].\n  repeat last change [count] times",
	"0": "0\n  move to beginning of line",
	";": ";\n  repeat last f/F/t/T",
	"<": "[count]<motion\n  shift lines left",
	">": "[count]>motion\n  shift lines right",
	"?": "?pattern\n  search backward for pattern",
	"@": "@buffer\n  execute vi commands from register",
	"A": "[count]A\n  append text at end of line",
	"B": "[count]B\n  move backward [count] WORDs",
	"C": "[count]C\n  change to end of line",
	"D": "[count]D\n  delete to end of line",
	"E": "[count]E\n  move to end of WORD",
	"F": "[count]F char\n  find character backward in line",
	"G": "[count]G\n  go to line [count] (default: last line)",
	"H": "[count]H\n  move to [count]th line from top of screen",
	"I": "[count]I\n  insert before first non-blank",
	"J": "[count]J\n  join [count] lines",
	"L": "[count]L\n  move to [count]th line from bottom of screen",
	"M": "M\n  move to middle screen line",
	"N": "[count]N\n  repeat last search in opposite direction",
	"O": "[count]O\n  open line above and insert",
	"P": "[count]P\n  put before cursor",
	"Q": "Q\n  enter line-oriented ex mode",
	"R": "R\n  enter replace mode",
	"S": "[count]S\n  substitute [count] lines",
	"T": "[count]T char\n  find backward, stopping before char",
	"U": "U\n  undo all changes on current line",
	"X": "[count]X\n  delete characters backward",
	"Y": "[count]Y\n  yank lines",
	"Z": "ZZ | ZQ\n  write and quit | quit without writing",
	"^": "^\n  move to first non-blank character on the line",
	"_": "_\n  move to first non-blank of last line",
	"a": "[count]a\n  append text after cursor",
	"b": "[count]b\n  move backward [count] words",
	"c": "[count]c motion\n  change text",
	"d": "[count]d motion\n  delete text",
	"e": "[count]e\n  move to end of word",
	"f": "[count]f char\n  find character forward in line",
	"g": "^G\n  display file information (use ^G)",
	"h": "[count]h\n  move left [count] characters",
	"i": "[count]i\n  insert before cursor",
	"j": "[count]j\n  move down [count] logical lines",
	"k": "[count]k\n  move up [count] logical lines",
	"l": "[count]l | [count]space\n  move right [count] characters",
	"m": "m{a-z}\n  set mark at cursor",
	"n": "[count]n\n  repeat last search [count] times",
	"o": "[count]o\n  open line below and insert",
	"p": "[count]p\n  put after cursor",
	"r": "[count]r char\n  replace [count] characters",
	"s": "[count]s\n  substitute [count] characters",
	"t": "[count]t char\n  find forward, stopping before char",
	"u": "[count]u\n  undo [count] changes",
	"w": "[count]w\n  move forward [count] words",
	"x": "[count]x\n  delete [count] characters",
	"y": "[count]y motion\n  yank text",
	"z": "[line]z[count][+-.<CR>]\n  reposition screen (z[count] types equivalent)",
	"{": "[count]{\n  move backward [count] paragraphs",
	"}": "[count]}\n  move forward [count] paragraphs",
	"|": "[count]|\n  move to column [count]",
	"~": "[count]~ | [count]~motion\n  toggle case",
	"`": "`{mark}\n  move to exact mark position",
	"/": "/pattern\n  search forward for pattern",
	":": ":\n  enter an ex command on the colon line",
}

func (e *Engine) exHelp(*exCmd) error {
	e.showOutput([]string{
		`To see the list of vi commands, enter ":viusage<CR>"`,
		`To see the list of ex commands, enter ":exusage<CR>"`,
		`For an ex command usage statement enter ":exusage [cmd]<CR>"`,
		`For a vi key usage statement enter ":viusage [key]<CR>"`,
		`To exit, enter ":q!"`,
	})
	return nil
}

func (e *Engine) exExusage(c *exCmd) error {
	arg := strings.TrimSpace(c.arg)
	if arg == "" {
		names := make([]string, 0, len(exCmds))
		seen := make(map[string]bool, len(exCmds))
		for _, d := range exCmds {
			if d.summary == "" || seen[d.full] {
				continue
			}
			seen[d.full] = true
			names = append(names, d.full)
		}
		sort.Strings(names)
		lines := make([]string, 0, len(names))
		for _, name := range names {
			d, err := findCmd(name)
			if err != nil {
				continue
			}
			lines = append(lines, fmt.Sprintf("%-16s%s", name+":", d.summary))
		}
		e.showOutput(lines)
		return nil
	}
	def, err := findCmd(arg)
	if err != nil {
		return err
	}
	if def.summary == "" {
		return fmt.Errorf("The %s command is unknown", arg)
	}
	usage := strings.ReplaceAll(def.usage, "\n", "\n         ")
	e.showOutput([]string{
		"Command: " + def.summary,
		"  Usage: " + usage,
	})
	return nil
}

func (e *Engine) exViusage(c *exCmd) error {
	arg := strings.TrimSpace(c.arg)
	if arg == "" {
		e.showOutput(append([]string(nil), viUsageList...))
		return nil
	}
	if len([]rune(arg)) != 1 {
		return fmt.Errorf("Usage: viusage [key]")
	}
	if arg == "[" {
		e.showOutput([]string{"[[", "  move backward one section"})
		return nil
	}
	if arg == "]" {
		e.showOutput([]string{"]]", "  move forward one section"})
		return nil
	}
	if detail, ok := viUsageDetail[arg]; ok {
		e.showOutput(strings.Split(detail, "\n"))
		return nil
	}
	return fmt.Errorf("The %s key has no current meaning", arg)
}
