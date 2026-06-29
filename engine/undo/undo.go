// Package undo provides multi-level undo/redo over a buffer.LineStore. Edits
// are made through a Log, which records each line mutation with the data needed
// to invert and to replay it. Mutations are grouped into change sets delimited
// by Begin/End; one Undo reverses the most recent change set and one Redo
// replays it, mirroring nvi's cursor-bracketed log records (common/log.c) but
// with an explicit redo stack.
package undo

import "govi/engine/buffer"

// Pos is a cursor position recorded with a change set so undo/redo can restore
// where the user was. Line is 1-based; Col is a 0-based rune index.
type Pos struct {
	Line int64
	Col  int
}

type recKind int

const (
	recSet recKind = iota
	recInsert
	recDelete
)

// rec captures one line mutation. before holds the prior line (set, delete);
// after holds the new line (set, insert).
type rec struct {
	kind   recKind
	lno    int64
	before []rune
	after  []rune
}

type changeset struct {
	recs   []rec
	before Pos // cursor before the change (undo lands here)
	after  Pos // cursor after the change (redo lands here)
}

// Log records edits to a LineStore and supports undo/redo.
type Log struct {
	store buffer.LineStore

	open    int // nesting depth; Begin/End nest so a command made of sub-commands
	cur     []rec // (e.g. :g) records one undo group
	curFrom Pos

	undo []changeset
	redo []changeset

	// gen counts content mutations. It increments on every store change
	// (including undo/redo) so callers can cheaply detect "did the buffer
	// content change since I last looked" -- e.g. to invalidate a display cache.
	gen uint64
}

// New returns a Log that edits store.
func New(store buffer.LineStore) *Log { return &Log{store: store} }

// Gen returns the current edit generation. It changes whenever any line content
// is mutated through this log (edits, undo, or redo).
func (l *Log) Gen() uint64 { return l.gen }

// Begin opens a change set; cursor is where the cursor was before the edits.
// Begin/End nest: a command implemented by running sub-commands (e.g. :g, which
// runs an ex command per matching line) wraps the whole run in an outer
// Begin/End so all the sub-edits collapse into a single undo group. Only the
// outermost Begin sets the starting cursor.
func (l *Log) Begin(cursor Pos) {
	if l.open == 0 {
		l.cur = nil
		l.curFrom = cursor
	}
	l.open++
}

// Pending reports whether a change set is open and has recorded mutations.
// The buffer may differ from the last saved copy even before End runs (e.g.
// while still in insert mode).
func (l *Log) Pending() bool { return l.open > 0 && len(l.cur) > 0 }

// End closes the current change set; cursor is where the cursor ended up. An
// empty change set (no mutations) is discarded. Any non-empty change set clears
// the redo stack. Inner (nested) Ends only decrement the depth; the outermost
// End commits the accumulated group.
func (l *Log) End(cursor Pos) {
	if l.open == 0 {
		return
	}
	l.open--
	if l.open > 0 {
		return
	}
	if len(l.cur) > 0 {
		l.undo = append(l.undo, changeset{recs: l.cur, before: l.curFrom, after: cursor})
		l.redo = l.redo[:0]
	}
	l.cur = nil
}

func clone(r []rune) []rune {
	out := make([]rune, len(r))
	copy(out, r)
	return out
}

// Set replaces line lno, recording the change.
func (l *Log) Set(lno int64, line []rune) {
	before, _ := l.store.Get(lno)
	l.record(rec{recSet, lno, clone(before), clone(line)})
	l.store.Set(lno, line)
	l.gen++
}

// Insert inserts line before lno, recording the change.
func (l *Log) Insert(lno int64, line []rune) {
	l.record(rec{recInsert, lno, nil, clone(line)})
	l.store.Insert(lno, line)
	l.gen++
}

// Append inserts line after lno, recording the change.
func (l *Log) Append(lno int64, line []rune) { l.Insert(lno+1, line) }

// Delete removes line lno, recording the change.
func (l *Log) Delete(lno int64) {
	before, _ := l.store.Get(lno)
	l.record(rec{recDelete, lno, clone(before), nil})
	l.store.Delete(lno)
	l.gen++
}

func (l *Log) record(r rec) {
	if l.open == 0 {
		// Defensive: an unbracketed edit becomes its own change set so it can
		// still be undone.
		l.cur = []rec{r}
		l.undo = append(l.undo, changeset{recs: l.cur})
		l.redo = l.redo[:0]
		l.cur = nil
		return
	}
	l.cur = append(l.cur, r)
}

// CanUndo reports whether there is a change set to undo.
func (l *Log) CanUndo() bool { return len(l.undo) > 0 }

// CanRedo reports whether there is a change set to redo.
func (l *Log) CanRedo() bool { return len(l.redo) > 0 }

// Undo reverses the most recent change set and returns the cursor position to
// restore. ok is false if there is nothing to undo.
func (l *Log) Undo() (cursor Pos, ok bool) {
	if len(l.undo) == 0 {
		return Pos{}, false
	}
	cs := l.undo[len(l.undo)-1]
	l.undo = l.undo[:len(l.undo)-1]
	for i := len(cs.recs) - 1; i >= 0; i-- {
		r := cs.recs[i]
		switch r.kind {
		case recSet:
			l.store.Set(r.lno, r.before)
		case recInsert:
			l.store.Delete(r.lno)
		case recDelete:
			l.store.Insert(r.lno, r.before)
		}
	}
	l.redo = append(l.redo, cs)
	l.gen++
	return cs.before, true
}

// UndoLineOnly undoes the most recent change set, but only if every record in
// it is on line lno. It returns the restore cursor and whether it undid
// anything. This backs vi's U command, which restores the current line by
// rolling back the changes confined to it.
func (l *Log) UndoLineOnly(lno int64) (cursor Pos, ok bool) {
	if len(l.undo) == 0 {
		return Pos{}, false
	}
	cs := l.undo[len(l.undo)-1]
	for _, r := range cs.recs {
		if r.lno != lno {
			return Pos{}, false
		}
	}
	return l.Undo()
}

// Redo replays the most recently undone change set and returns the cursor
// position to restore. ok is false if there is nothing to redo.
func (l *Log) Redo() (cursor Pos, ok bool) {
	if len(l.redo) == 0 {
		return Pos{}, false
	}
	cs := l.redo[len(l.redo)-1]
	l.redo = l.redo[:len(l.redo)-1]
	for _, r := range cs.recs {
		switch r.kind {
		case recSet:
			l.store.Set(r.lno, r.after)
		case recInsert:
			l.store.Insert(r.lno, r.after)
		case recDelete:
			l.store.Delete(r.lno)
		}
	}
	l.undo = append(l.undo, cs)
	l.gen++
	return cs.after, true
}
