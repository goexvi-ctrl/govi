package tips

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseBasic(t *testing.T) {
	tbl := Parse("" +
		"# comment\n" +
		"malloc calloc\n" +
		"    Allocate dynamic memory.\n" +
		"    Returns NULL on failure.\n" +
		"\n" +
		"free\n" +
		"\tRelease memory.\n")
	want := "Allocate dynamic memory.\nReturns NULL on failure."
	if tbl["malloc"] != want {
		t.Errorf("malloc = %q, want %q", tbl["malloc"], want)
	}
	if tbl["calloc"] != want {
		t.Errorf("calloc = %q, want %q (shared term line)", tbl["calloc"], want)
	}
	if tbl["free"] != "Release memory." {
		t.Errorf("free = %q", tbl["free"])
	}
	if _, ok := tbl["Allocate"]; ok {
		t.Error("body words must not become terms")
	}
}

func TestParseDedentKeepsRelativeIndent(t *testing.T) {
	tbl := Parse("" +
		"free\n" +
		"    Release memory. Example:\n" +
		"        free(p);\n")
	want := "Release memory. Example:\n    free(p);"
	if tbl["free"] != want {
		t.Errorf("free = %q, want %q", tbl["free"], want)
	}
}

func TestParseInteriorBlankAndTrailingBlank(t *testing.T) {
	tbl := Parse("" +
		"word\n" +
		"  first paragraph\n" +
		"\n" +
		"  second paragraph\n" +
		"\n" +
		"\n" +
		"other\n" +
		"  x\n" +
		"\n")
	if tbl["word"] != "first paragraph\n\nsecond paragraph" {
		t.Errorf("word = %q", tbl["word"])
	}
	if tbl["other"] != "x" {
		t.Errorf("other = %q", tbl["other"])
	}
}

func TestParseIndentedCommentIsBody(t *testing.T) {
	tbl := Parse("" +
		"word\n" +
		"  code:\n" +
		"    # not a comment here\n" +
		"# but this is\n" +
		"next\n" +
		"  n\n")
	if tbl["word"] != "code:\n  # not a comment here" {
		t.Errorf("word = %q", tbl["word"])
	}
	if tbl["next"] != "n" {
		t.Errorf("next = %q", tbl["next"])
	}
}

func TestParseLenient(t *testing.T) {
	// Indented text before any term is ignored; duplicates keep the later
	// entry; a bodyless term line removes the word.
	tbl := Parse("" +
		"  stray indented line\n" +
		"word\n" +
		"  old\n" +
		"word\n" +
		"  new\n" +
		"gone\n" +
		"  defined\n" +
		"gone\n")
	if len(tbl) != 1 || tbl["word"] != "new" {
		t.Errorf("table = %#v, want just word=new", tbl)
	}
}

func TestParseEmpty(t *testing.T) {
	if tbl := Parse(""); len(tbl) != 0 {
		t.Errorf("empty source parsed to %#v", tbl)
	}
	if tbl := Parse("# only comments\n\n"); len(tbl) != 0 {
		t.Errorf("comment-only source parsed to %#v", tbl)
	}
}

func TestCacheReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.tips")
	if err := os.WriteFile(p, []byte("a\n  one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var c Cache
	if got := c.Get(p); got["a"] != "one" {
		t.Fatalf("first load = %#v", got)
	}
	// Unchanged file: still served (from the cache path).
	if again := c.Get(p); again["a"] != "one" {
		t.Fatalf("cached load = %#v", again)
	}
	// Rewrite with a bumped mtime: Get picks up the new content.
	if err := os.WriteFile(p, []byte("a\n  two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if got := c.Get(p); got["a"] != "two" {
		t.Fatalf("reload = %#v, want a=two", got)
	}
}

func TestCacheMissingAndEmptyPath(t *testing.T) {
	var c Cache
	if got := c.Get(""); got != nil {
		t.Errorf("empty path = %#v, want nil", got)
	}
	if got := c.Get(filepath.Join(t.TempDir(), "nope.tips")); got != nil {
		t.Errorf("missing file = %#v, want nil", got)
	}
}
