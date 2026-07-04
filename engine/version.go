package engine

import "strings"

// Build metadata. Release builds inject commitDate, commitHash, and treeState
// via -ldflags (see scripts/version-ldflags.sh).
var (
	versionName = "govi-0.4.0"
	commitDate  = "unknown" // date of HEAD commit (YYYY-MM-DD); hash disambiguates same-day commits
	commitHash  = ""
	treeState   = "" // "" when clean; "modified" when the tree has local changes
	buildTime   = "" // UTC timestamp when built from a dirty tree (dev builds only)
)

// VersionString returns the :version message shown to the user.
func VersionString() string {
	var b strings.Builder
	b.WriteString("Version ")
	b.WriteString(versionName)
	b.WriteString(" (")
	b.WriteString(commitDate)
	b.WriteByte(')')
	if commitHash != "" {
		b.WriteByte(' ')
		b.WriteString(commitHash)
	}
	if treeState != "" {
		b.WriteByte(' ')
		b.WriteString(treeState)
		if buildTime != "" {
			b.WriteByte(' ')
			b.WriteString(buildTime)
		}
	}
	return b.String()
}

func (e *Engine) exVersion(*exCmd) error {
	e.scr.msg = VersionString()
	e.scr.msgKind = MsgInfo
	return nil
}
