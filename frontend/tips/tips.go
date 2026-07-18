// Package tips parses GoVi tooltip files: the word -> tooltip-text table
// behind GoVi.app's hover/manual tooltips (the tooltipfile option). Pure Go
// with no engine dependency so the format is unit-testable on its own.
//
// The format is line-based and indentation-driven:
//
//	# GoVi tooltip file. '#' in column 0 is a comment.
//	#
//	# An entry is one unindented term line -- one or more words separated by
//	# whitespace, all sharing the same tooltip -- followed by the tooltip
//	# body on indented lines.
//	malloc calloc
//	    Allocate dynamic memory.
//	    Returns NULL on failure.
//
//	free
//	    Release memory obtained from malloc()/calloc().
//	    Example:
//	        free(p);
//
// Rules:
//   - '#' in column 0 is a comment anywhere; an indented '#' line inside a
//     body is body text (so code samples keep their comments).
//   - The body is every following line that starts with a space or tab. The
//     body's common leading indentation is stripped, so deeper indentation
//     (like the free(p) example) survives relative to the rest.
//   - Blank lines inside a body are kept; blank lines between entries and
//     trailing blank lines are dropped.
//   - Words match exactly (case-sensitive), as selected by the editor's
//     double-click word boundaries.
//   - A word defined twice keeps the later entry; a term line with no body
//     removes the words' tooltips.
//
// The parser is deliberately lenient -- it never fails. Indented text before
// any term line is ignored.
package tips

import "strings"

// Table maps a word to its tooltip text.
type Table map[string]string

// Parse builds a Table from tooltip-file source text.
func Parse(src string) Table {
	t := Table{}
	var terms []string
	var body []string
	flush := func() {
		if terms == nil {
			return
		}
		text := dedent(body)
		for _, w := range terms {
			if text == "" {
				delete(t, w)
			} else {
				t[w] = text
			}
		}
		terms, body = nil, nil
	}
	for _, ln := range strings.Split(src, "\n") {
		switch {
		case strings.HasPrefix(ln, "#"):
			// column-0 comment
		case strings.TrimSpace(ln) == "":
			if terms != nil {
				body = append(body, "") // interior blanks kept; dedent trims trailing ones
			}
		case ln[0] == ' ' || ln[0] == '\t':
			if terms != nil {
				body = append(body, ln)
			}
		default:
			flush()
			terms = strings.Fields(ln)
		}
	}
	flush()
	return t
}

// dedent joins body lines, stripping their common leading whitespace and any
// trailing blank lines. Blank lines stay blank and do not constrain the
// common prefix.
func dedent(body []string) string {
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	if len(body) == 0 {
		return ""
	}
	prefix, first := "", true
	for _, ln := range body {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		ws := ln[:len(ln)-len(strings.TrimLeft(ln, " \t"))]
		if first {
			prefix, first = ws, false
		} else {
			prefix = commonPrefix(prefix, ws)
		}
	}
	out := make([]string, len(body))
	for i, ln := range body {
		if strings.TrimSpace(ln) == "" {
			continue // out[i] stays ""
		}
		out[i] = ln[len(prefix):]
	}
	return strings.Join(out, "\n")
}

func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}
