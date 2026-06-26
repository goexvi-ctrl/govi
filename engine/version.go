package engine

import "strings"

// Build metadata. Release builds inject commitTime, commitHash, and treeState
// via -ldflags (see scripts/version-ldflags.sh).
var (
	versionName = "govi-0.1"
	commitTime  = "unknown" // ISO timestamp of HEAD (git log -1 --format=%cI)
	commitHash  = ""
	treeState   = "" // "" when clean; "modified" when the tree has local changes
	buildTime   = "" // UTC timestamp when built from a dirty tree
)

// VersionString returns the :version message shown to the user.
func VersionString() string {
	var b strings.Builder
	b.WriteString("Version ")
	b.WriteString(versionName)
	b.WriteString(" (")
	b.WriteString(commitTime)
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
