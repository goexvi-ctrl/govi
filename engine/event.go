package engine

// Event is an input delivered to the engine via Engine.Input. It models nvi's
// EVENT (key.h) but as a Go sum type. Hosts construct concrete events from
// their own input source (terminal keys, GUI key events, IPC) so the engine
// never reads a device directly.
type Event interface{ isEvent() }

// Mod is a set of modifier keys held during a KeyEvent. Plain typed text
// arrives as a KeyEvent with Mods == 0 (or as a StringEvent for pasted runs).
type Mod uint8

const (
	ModCtrl Mod = 1 << iota
	ModAlt
	ModShift // typically only meaningful with SpecialKey events
)

// SpecialKey names a non-text key. KeyNone means the event carries a rune in
// KeyEvent.Rune instead.
type SpecialKey int

const (
	KeyNone SpecialKey = iota
	KeyEscape
	KeyEnter
	KeyTab
	KeyBackspace
	KeyDelete
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyInsert
	KeyFunc // function key; FuncN carries the number
)

// KeyEvent is a single keypress. If Key != KeyNone it is a named key and Rune
// is ignored; otherwise Rune is the typed character. Mods carries modifiers,
// and FuncN is the function-key number when Key == KeyFunc.
type KeyEvent struct {
	Rune  rune
	Key   SpecialKey
	Mods  Mod
	FuncN int
}

func (KeyEvent) isEvent() {}

// StringEvent delivers a run of literal text at once, e.g. a bracketed paste or
// the expansion of a :map. The engine treats it as if the runes were typed.
type StringEvent struct {
	Text string
}

func (StringEvent) isEvent() {}

// ResizeEvent reports new viewport geometry. Engine.Resize is the usual entry
// point; ResizeEvent exists so geometry changes can be queued in the same
// stream as keys when that is convenient for a host.
type ResizeEvent struct {
	Rows, Cols int
}

func (ResizeEvent) isEvent() {}

// InterruptEvent corresponds to the user's interrupt (SIGINT / typed ^C),
// cancelling an in-progress command or input.
type InterruptEvent struct{}

func (InterruptEvent) isEvent() {}

// SuspendEvent requests job-control suspend (^Z). Terminal hosts honor it; GUI
// hosts typically ignore it.
type SuspendEvent struct{}

func (SuspendEvent) isEvent() {}

// TimeoutEvent fires when an input wait elapses, used to resolve ambiguous map
// prefixes (the keytimeout/timeout options).
type TimeoutEvent struct{}

func (TimeoutEvent) isEvent() {}
