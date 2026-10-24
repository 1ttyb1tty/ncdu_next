package scan

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"
)

// FSAvail is the three-state availability of a filesystem's type. Every
// constant names its type explicitly (the AST guard in internal/ui reads them by
// type and cannot see a constant written with the `= "x"` shorthand).
type FSAvail string

const (
	FSAvailable     FSAvail = "available"      // OS named the filesystem
	FSUnavailable   FSAvail = "unavailable"    // read failed, path not covered, or unimplemented here
	FSNotApplicable FSAvail = "not-applicable" // nothing to read: the scan root is a regular file
)

// FSAvailValues is the canonical enum order, beside the const block so the
// Debug tab's reference and the AST guard test share one source of truth.
func FSAvailValues() []FSAvail { return []FSAvail{FSAvailable, FSUnavailable, FSNotApplicable} }

// FSInfo describes one filesystem as the OS reports it. It is a record (see the
// Debug tab) and every field carries a `doc` tag.
type FSInfo struct {
	MountPoint string  `doc:"mount point / volume root of this filesystem"`
	FSType     string  `doc:"OS-reported filesystem name (NTFS, ext4, apfs); empty unless Avail is available"`
	Avail      FSAvail `doc:"three-state availability: available | unavailable | not-applicable"`
	Device     uint64  `doc:"opaque OS identity for this filesystem (Linux major<<32|minor, Windows volume serial, darwin Fsid); 0 when the OS reports none; never compared across platforms"`
	Reason     string  `doc:"why the type is unavailable or not applicable; empty when available"`
}

// FSReport is the per-scan filesystem summary exposed by Walker.FSReport(). It
// is transient scan output like Walker, not a record.
type FSReport struct {
	Root   FSInfo   // the scan root's filesystem (or its unavailability reason)
	Mounts []FSInfo // mount points strictly inside the root
}

// errUnsupported is returned by osMountTable on platforms with no reader.
var errUnsupported = errors.New("filesystem mount table not implemented on this platform")

// Platform seams. They are package vars so a test can stub the reader or the
// --one-file-system identity on any platform (including the Windows-only CI).
// osMountTable takes the absolute scan root so platforms that must ask the OS
// about a volume (Windows) can limit device-backed queries to volumes that
// actually cover the root.
var (
	osMountTableFn  = osMountTable
	deviceOfEntryFn = deviceOfEntry
)

// parseMountinfo parses Linux /proc/self/mountinfo. Each line is space-separated:
// mount id, parent id, major:minor, root, mount point, mount options, zero or
// more optional fields, a "-" separator, then fs type and mount source. Mount
// points carry octal escapes (\040 space, \011 tab, \012 newline, \134
// backslash). Device packs the major/minor numbers opaquely as major<<32|minor.
func parseMountinfo(r io.Reader) ([]FSInfo, error) {
	var out []FSInfo
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 7 {
			continue
		}
		sep := -1
		for i, f := range fields {
			if f == "-" {
				sep = i
				break
			}
		}
		if sep < 0 || sep+2 > len(fields) {
			continue
		}
		dev, ok := parseDevice(fields[2])
		if !ok {
			dev = 0
		}
		out = append(out, FSInfo{
			MountPoint: unescapeMount(fields[4]),
			FSType:     fields[sep+1],
			Avail:      FSAvailable,
			Device:     dev,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// parseDevice parses a "major:minor" pair and packs it as major<<32|minor.
func parseDevice(s string) (uint64, bool) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return 0, false
	}
	major, err1 := strconv.ParseUint(s[:i], 10, 32)
	minor, err2 := strconv.ParseUint(s[i+1:], 10, 32)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return major<<32 | minor, true
}

// unescapeMount decodes the octal escapes mountinfo uses for whitespace and
// backslash in mount points.
func unescapeMount(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// samePath reports whether two paths name the same location, via mutual
// pathPrefix so it inherits the platform's separator and case rules.
func samePath(a, b string) bool {
	return pathPrefix(a, b) && pathPrefix(b, a)
}

// mountsUnder returns the entries of table whose mount point is strictly inside
// root (the root's own entry is excluded).
func mountsUnder(root string, table []FSInfo) []FSInfo {
	var out []FSInfo
	for _, m := range table {
		if m.MountPoint == "" || samePath(root, m.MountPoint) {
			continue
		}
		if pathPrefix(root, m.MountPoint) {
			out = append(out, m)
		}
	}
	return out
}

// fsFor returns the table entry with the longest mount point that covers path.
func fsFor(path string, table []FSInfo) (FSInfo, bool) {
	best := -1
	for i, m := range table {
		if m.MountPoint == "" || !pathPrefix(m.MountPoint, path) {
			continue
		}
		if best < 0 || len(m.MountPoint) > len(table[best].MountPoint) {
			best = i
		}
	}
	if best < 0 {
		return FSInfo{}, false
	}
	return table[best], true
}

// FSLookup resolves the filesystem covering path: the longest mount under the
// root, else the root's own filesystem. A nil report has no answer; the UI must
// not reimplement path semantics, so it goes through here.
func FSLookup(fs *FSReport, path string) (FSInfo, bool) {
	if fs == nil {
		return FSInfo{}, false
	}
	if info, ok := fsFor(path, fs.Mounts); ok {
		return info, true
	}
	return fs.Root, true
}
