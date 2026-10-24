package ui

import (
	"fmt"

	"github.com/1ttyb1tty/ncdu-next/internal/scan"
)

// filesystemSections returns the `filesystem` section shown in the Debug page's
// `tab` zone for the inspected tab. It is inserted right after the per-tab
// detail, so full state and the reason live in the reference page while the
// navigation header carries only a compact token.
func filesystemSections(inspect *Tab) []debugSection {
	if inspect == nil {
		return nil
	}
	return []debugSection{{Zone: "tab", Title: "filesystem", Lines: filesystemLines(inspect)}}
}

// fsToken returns the compact filesystem token shown in the navigation header
// for the directory currently viewed: "[NTFS]" when the OS named the filesystem,
// "[fs:?]" when the type could not be read, and "" when it is not applicable or
// there is no report yet.
func (t *Tab) fsToken() string {
	if t.fs == nil || t.cur == nil {
		return ""
	}
	info, ok := scan.FSLookup(t.fs, t.cur.Path)
	if !ok {
		return ""
	}
	switch info.Avail {
	case scan.FSAvailable:
		name := sanitize(truncateWidth(info.FSType, 12))
		if name == "" {
			return "[fs:?]"
		}
		return "[" + name + "]"
	case scan.FSUnavailable:
		return "[fs:?]"
	default:
		return ""
	}
}

// filesystemLines renders the root filesystem, mount count, reason, and the
// per-mount table. OS-supplied strings (type, mount points, reason) are external
// data, so they are sanitized and truncated before entering the frame.
func filesystemLines(t *Tab) []string {
	if t.fs == nil {
		return []string{"  (no scan report)"}
	}
	r := t.fs.Root
	lines := []string{
		fmt.Sprintf("  root mount   %s", sanitizeOrDash(r.MountPoint)),
		fmt.Sprintf("  root fs      %s", fsStateText(r)),
		fmt.Sprintf("  mounts       %d", len(t.fs.Mounts)),
	}
	if r.Reason != "" {
		lines = append(lines, "  reason       "+sanitize(r.Reason))
	}
	if len(t.fs.Mounts) == 0 {
		lines = append(lines, "  (no nested mounts)")
		return lines
	}
	lines = append(lines, fmt.Sprintf("  %-28s %-10s %-13s %s", "mount point", "fstype", "avail", "device"))
	for _, m := range t.fs.Mounts {
		lines = append(lines, fmt.Sprintf("  %-28s %-10s %-13s %d",
			truncateWidth(sanitize(m.MountPoint), 28),
			truncateWidth(sanitize(m.FSType), 10),
			string(m.Avail),
			m.Device))
	}
	return lines
}

// fsStateText renders the root's availability: the type name when available, or
// the enum's own value for the other states, so a rename of an FSAvail constant
// cannot drift from this line, the mount-table column, and the bundle.
func fsStateText(info scan.FSInfo) string {
	switch info.Avail {
	case scan.FSAvailable:
		return sanitizeOrDash(info.FSType)
	case scan.FSUnavailable, scan.FSNotApplicable:
		return string(info.Avail)
	default:
		return "unknown"
	}
}

// sanitizeOrDash renders an empty string as "-" so a missing mount point is
// visible rather than blank.
func sanitizeOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return sanitize(s)
}
