package mark

import "testing"

func TestSetGetClear(t *testing.T) {
	s := New()
	if _, ok := s.Get('a'); ok {
		t.Fatal("unset mark should not be ok")
	}
	s.Set('a', Mark{Line: 5, Col: 3})
	mk, ok := s.Get('a')
	if !ok || mk.Line != 5 || mk.Col != 3 {
		t.Fatalf("got %+v ok=%v", mk, ok)
	}
	s.Clear('a')
	if _, ok := s.Get('a'); ok {
		t.Fatal("cleared mark should not be ok")
	}
}

func TestLinesInserted(t *testing.T) {
	s := New()
	s.Set('a', Mark{Line: 10})
	s.Set('b', Mark{Line: 3})
	s.Set('c', Mark{Line: 5}) // exactly at insertion point

	s.LinesInserted(5, 2) // two lines inserted before line 5

	if mk, _ := s.Get('a'); mk.Line != 12 {
		t.Fatalf("a.Line = %d, want 12", mk.Line)
	}
	if mk, _ := s.Get('b'); mk.Line != 3 {
		t.Fatalf("b.Line = %d, want 3", mk.Line)
	}
	if mk, _ := s.Get('c'); mk.Line != 7 {
		t.Fatalf("c.Line = %d, want 7", mk.Line)
	}
}

func TestLinesDeleted(t *testing.T) {
	s := New()
	s.Set('a', Mark{Line: 2}) // above the deleted range
	s.Set('o', Mark{Line: 4}) // on a deleted line
	s.Set('b', Mark{Line: 8}) // below the deleted range

	s.LinesDeleted(4, 3) // remove lines 4,5,6

	if mk, _ := s.Get('a'); mk.Line != 2 {
		t.Fatalf("above.Line = %d, want 2", mk.Line)
	}
	if _, ok := s.Get('o'); ok {
		t.Fatal("mark on deleted line should be invalid")
	}
	if mk, _ := s.Get('b'); mk.Line != 5 {
		t.Fatalf("below.Line = %d, want 5 (shifted up by 3)", mk.Line)
	}
}

func TestResetDeletedMark(t *testing.T) {
	s := New()
	s.Set('a', Mark{Line: 4})
	s.LinesDeleted(4, 1)
	if _, ok := s.Get('a'); ok {
		t.Fatal("expected deleted")
	}
	s.Set('a', Mark{Line: 1}) // re-setting clears deleted state
	if mk, ok := s.Get('a'); !ok || mk.Line != 1 {
		t.Fatalf("re-set mark: got %+v ok=%v", mk, ok)
	}
}
