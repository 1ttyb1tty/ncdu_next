package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/debuginfo"
	"github.com/1ttyb1tty/ncdu-next/internal/scan"
)

// debugInfoMsg carries the result of the E key's bundle write.
type debugInfoMsg struct {
	path string
	err  error
}

// writeDebugInfoCmd writes the default-named bundle off the UI thread, so a
// large dump cannot stall rendering. It never includes the full tree: only the
// --debuginfo-tree flag can.
func (m Model) writeDebugInfoCmd() tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		in := m.DebugInfoInput(false, debuginfo.BuildInfoText(), now)
		dest := debuginfo.DefaultName(now)
		if err := debuginfo.Write(dest, in); err != nil {
			return debugInfoMsg{err: err}
		}
		return debugInfoMsg{path: dest}
	}
}

// DebugPageText returns the full two-zone Debug page as text, so the headless
// bundle path does not need a running TUI.
func (m Model) DebugPageText() string {
	return strings.Join(debugPage(debugSections(m.tabs, m.inspect)), "\n")
}

// DebugInfoInput maps this model's tabs, filesystem reports and trees into the
// debuginfo bundle input. now, build info and the tree switch come from the
// caller so the write is reproducible.
func (m Model) DebugInfoInput(includeTree bool, buildInfo string, now time.Time) debuginfo.Input {
	in := debuginfo.Input{
		Options:     m.cfg,
		DebugPage:   m.DebugPageText(),
		BuildInfo:   buildInfo,
		Runtime:     strings.Join(runtimeLines(), "\n"),
		IncludeTree: includeTree,
		Now:         now,
	}
	for _, t := range m.tabs {
		in.Tabs = append(in.Tabs, tabRecord(t))
		if t.root != nil {
			in.Trees = append(in.Trees, debuginfo.TabTree{TabIndex: t.displayNumber(), Tree: t.root})
		}
		in.Mounts = append(in.Mounts, mountRecords(t)...)
	}
	return in
}

func tabRecord(t *Tab) debuginfo.TabRecord {
	r := debuginfo.TabRecord{
		Index:        t.displayNumber(),
		Kind:         string(t.kind),
		Status:       string(t.status),
		Root:         t.rootPath,
		NodeCount:    t.nodeCount,
		EstMemBytes:  nodeRecSize() * uint64(t.nodeCount),
		Unit:         unitLabel(t.unit),
		Expanded:     t.expanded,
		DetectFormat: t.detectFormat,
		FilterError:  t.filterErr,
		SearchActive: t.searchActive,
		SearchRaw:    t.searchRaw,
		SearchCount:  len(t.searchResults),
	}
	if t.q != nil && t.q.Active() {
		r.FilterScope = "local"
		if t.deep {
			r.FilterScope = "deep"
		}
		r.FilterRaw = t.filterRaw
	}
	return r
}

// mountRecords flattens a tab's FSReport into bundle records: the root
// filesystem first, then each nested mount.
func mountRecords(t *Tab) []debuginfo.MountRecord {
	if t.fs == nil {
		return nil
	}
	idx := t.displayNumber()
	var out []debuginfo.MountRecord
	add := func(m scan.FSInfo) {
		out = append(out, debuginfo.MountRecord{
			TabIndex:   idx,
			MountPoint: m.MountPoint,
			FSType:     m.FSType,
			Avail:      string(m.Avail),
			Device:     m.Device,
			Reason:     m.Reason,
		})
	}
	add(t.fs.Root)
	for _, m := range t.fs.Mounts {
		add(m)
	}
	return out
}
