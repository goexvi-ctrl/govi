// Package engine is the embeddable vi/ex editor core. It has no terminal,
// curses, or GUI dependencies: all interaction with the outside world crosses
// the Frontend / View boundary defined in this file.
//
// A host (terminal program, GUI application, test harness) creates an Engine,
// feeds it input via Engine.Input and geometry via Engine.Resize, and renders
// whatever the engine exposes through the read-only View. The engine pushes a
// ChangeSet to the Frontend whenever presentation state changes so incremental
// renderers can avoid full repaints.
//
// This mirrors nvi's GS->scr_* function-pointer table (common/gs.h), but turns
// the "push cells at a terminal" model into a semantic document model: the
// engine publishes buffer text, cursor, mode, viewport, and messages as data,
// and the frontend decides how to draw them.
package engine

// Mode is the editor's current interaction mode, as the user perceives it.
type Mode int

const (
	ModeCommand Mode = iota // vi command mode
	ModeInsert              // vi insert mode (showmode "Insert")
	ModeReplace             // vi replace mode (R), showmode "Replace"
	ModeExColon             // colon line is being entered from vi
	ModeExText              // line-oriented ex mode (the "ex"/Q editor)
)

// Pos is a position in the buffer. Line is 1-based to match vi's line numbers;
// Col is a 0-based rune index within the line (not a display column).
type Pos struct {
	Line int64
	Col  int
}

// Viewport describes what the engine wants shown and the geometry it was given.
// The engine owns Top because vi scrolling commands (^F, ^B, z, H/M/L, ^E/^Y)
// are part of editor semantics; a GUI may scroll smoothly but must keep Top as
// the logical first visible line for those commands to behave correctly.
type Viewport struct {
	Top     int64 // first buffer line the engine wants visible (1-based)
	Rows    int   // full text rows available, as last set via Engine.Resize
	MapRows int   // active map height (nvi t_rows); 0 means use Rows
	Cols    int   // columns available, as last set via Engine.Resize
}

// MessageKind classifies the status/message line content, mirroring nvi's
// mtype_t (msg.h).
type MessageKind int

const (
	MsgNone MessageKind = iota
	MsgInfo
	MsgError
	MsgBell // informational, frontend may also ring the bell
)

// Style is a display attribute applied to a run of runes within a DisplayLine.
type Style uint8

const (
	StyleNormal  Style = 0
	StyleReverse Style = 1 << iota // inverse video (e.g. search highlight)
	StyleBold
	StyleUnderline
)

// Span applies a Style to runes [Start, End) (rune indices) of a DisplayLine.
type Span struct {
	Start, End int
	Style      Style
}

// DisplayLine is one buffer line presented for rendering. Text holds the raw
// runes; Widths[i] is the display width (cells) of Text[i] so frontends can do
// column math without a wcwidth of their own. Spans carries any styling. A nil
// Spans means the whole line is StyleNormal.
type DisplayLine struct {
	Text   []rune
	Widths []int8
	Spans  []Span
	List   bool // :set list; a trailing $ is shown after Text
}

// View is the read-only semantic document model the frontend renders. All
// methods are valid to call only from within Frontend.Render (or while the
// host otherwise holds the engine quiescent); the engine does not mutate state
// concurrently with a Render call.
type View interface {
	// LineCount returns the number of lines in the active buffer (>= 1).
	LineCount() int64
	// Line returns display data for 1-based line lno.
	Line(lno int64) DisplayLine
	// Cursor returns the current cursor position.
	Cursor() Pos
	// Mode returns the current interaction mode.
	Mode() Mode
	// Viewport returns the engine's desired top line and current geometry.
	Viewport() Viewport
	// Message returns the status/message line text and its kind.
	Message() (text string, kind MessageKind)
	// Name returns the display name of the active buffer (file path or "").
	Name() string
	// Modified reports whether the active buffer has unsaved changes.
	Modified() bool
	// Number reports whether line numbers should be shown (:set number).
	Number() bool
	// List reports whether list mode is on (:set list).
	List() bool
	// ExTranscript returns the scrolling ex-mode output lines; non-nil only
	// while Mode() == ModeExText.
	ExTranscript() []string
	// PendingOutput returns the current page of multi-line command output shown
	// as a screen overlay; nil when there is none.
	PendingOutput() []string
	// PendingOutputPrompt is the continue message on the overlay's last row.
	PendingOutputPrompt() string
	// MatchHighlight returns the position of the matching bracket to flash
	// (showmatch) and whether one is active; while active the frontend shows the
	// cursor there instead of at the insertion point.
	MatchHighlight() (Pos, bool)
}

// LineRange is an inclusive 1-based range of buffer lines.
type LineRange struct {
	First, Last int64
}

// ChangeSet is an incremental-render hint passed to Frontend.Render describing
// what changed since the previous render. A frontend may ignore the hints and
// repaint everything; they exist so a GUI can repaint minimally.
type ChangeSet struct {
	Full           bool        // repaint everything
	DirtyLines     []LineRange // buffer lines whose content changed
	CursorMoved    bool
	ModeChanged    bool
	MessageChanged bool
	Scrolled       bool // Viewport.Top changed
}

// ShellRunner is implemented by frontends that can run an interactive shell for
// :shell. The engine passes the shell program from the shell option; inExMode is
// true when ex line mode already owns the terminal (no extra suspend needed).
// GUI hosts may open an external terminal or report that the command is
// unavailable.
type ShellRunner interface {
	RunShell(shell string, inExMode bool) error
}

// BangRunner runs :!cmd when no line range is given. Tests may stub this; hosts
// normally let the engine capture output in a pty (bangpty_unix.go).
type BangRunner interface {
	RunBang(shell, cmd, cwd string, cols, rows int) (output string, err error)
}

// Suspender is implemented by terminal frontends that can job-control suspend
// (^Z, :suspend, :stop). GUI hosts do not implement this.
type Suspender interface {
	Suspend() error
}

// Frontend is implemented by the host. The engine calls these methods
// synchronously from the goroutine that drives Engine.Input; a GUI host should
// marshal the work onto its UI thread. A Frontend must never call back into the
// Engine from within these methods.
type Frontend interface {
	// Render is invoked when presentation state changes. The frontend pulls
	// what it needs from v, guided by cs.
	Render(v View, cs ChangeSet)
	// Bell signals an error/alert (visual or audible).
	Bell()
	// SetTitle updates the host window/title (xterm title, GUI titlebar).
	SetTitle(title string)
}
