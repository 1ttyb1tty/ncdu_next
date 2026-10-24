package tree

import (
	"path/filepath"
	"sort"
)

// Node is the per-entry record for both files and directories. The struct is
// shared; IsDir selects the mode, and the meaning of Children/Parent/Size
// differs by mode. The doc/file/dir struct tags are read by the Debug tab's
// record reference (internal/ui/recinfo.go) and guarded by tests there.
//
// Field order is chosen for size: the three int64 timestamps sit next to the
// other 8-byte fields and the small scalars (IsDir/Kind/Attrs) sit at the end,
// so a densely packed tree does not pay avoidable padding per node. Access is by
// name everywhere, so the order is not part of any interface.
type Node struct {
	Name       string  `doc:"base name of the entry" file:"set" dir:"set"`
	Path       string  `doc:"full path from the scan root" file:"set" dir:"set"`
	Size       int64   `doc:"bytes on disk; for a directory, the recomputed subtree total" file:"bytes" dir:"subtree"`
	AppSize    int64   `doc:"sum of apparent sizes" file:"bytes" dir:"sum"`
	Mtime      int64   `doc:"modification time, Unix epoch nanoseconds (0 if unread)" file:"set" dir:"set"`
	Ctime      int64   `doc:"change/creation time, Unix epoch nanoseconds (0 if unread)" file:"maybe" dir:"maybe"`
	Atime      int64   `doc:"access time, Unix epoch nanoseconds (0 if unread)" file:"maybe" dir:"maybe"`
	Children   []*Node `doc:"child records" file:"nil" dir:"array"`
	Parent     *Node   `doc:"owning directory" file:"set" dir:"set|nil"`
	Type       string  `doc:"specific format id from the extension table (see enums)" file:"set" dir:""`
	Encoding   string  `doc:"text encoding, populated under --detect-format" file:"maybe" dir:"maybe"`
	LineEnding string  `doc:"crlf|lf|cr|mixed, populated under --detect-format" file:"maybe" dir:"maybe"`
	Err        error   `doc:"stat/read error for this entry" file:"maybe" dir:"maybe"`
	IsDir      bool    `doc:"selects directory vs file mode" file:"false" dir:"true"`
	Kind       Kind    `doc:"broad category derived from Type (see enums)" file:"set" dir:"zero"`
	Attrs      uint32  `doc:"Windows FILE_ATTRIBUTE_* bitmask; 0 on non-Windows" file:"maybe" dir:"maybe"`
}

func (n *Node) AddChild(c *Node) {
	c.Parent = n
	n.Children = append(n.Children, c)
}

func (n *Node) Recompute() {
	if !n.IsDir {
		return
	}
	var total int64
	for _, c := range n.Children {
		c.Recompute()
		total += c.Size
	}
	n.Size = total
	n.AppSize = total
}

func SortBySize(n *Node, desc bool) {
	if n == nil || !n.IsDir {
		return
	}
	sort.Slice(n.Children, func(i, j int) bool {
		if desc {
			return n.Children[i].Size > n.Children[j].Size
		}
		return n.Children[i].Size < n.Children[j].Size
	})
	for _, c := range n.Children {
		SortBySize(c, desc)
	}
}

func SortByName(n *Node) {
	if n == nil || !n.IsDir {
		return
	}
	sort.Slice(n.Children, func(i, j int) bool {
		return n.Children[i].Name < n.Children[j].Name
	})
	for _, c := range n.Children {
		SortByName(c)
	}
}

func (n *Node) Delete() {
	if n.Parent == nil {
		return
	}
	for i, c := range n.Parent.Children {
		if c == n {
			n.Parent.Children = append(n.Parent.Children[:i], n.Parent.Children[i+1:]...)
			break
		}
	}
	cur := n.Parent
	for cur != nil {
		cur.Recompute()
		cur = cur.Parent
	}
}

func BaseName(p string) string { return filepath.Base(p) }

// Kind is the broad category of a scanned entry, derived from its Type id. It is
// a narrow uint8 enum: a small fixed set, cheap to compare across many nodes.
type Kind uint8

const (
	KindUnknown Kind = iota
	KindDiskImage
	KindContainerImage
	KindMemoryState
	KindOfficeDoc
	KindImage
	KindAudio
	KindVideo
	KindText
	KindSource
	KindMarkup
	KindOther
)

// kindNames holds the canonical name of each Kind, indexed by value.
var kindNames = [...]string{
	KindUnknown:        "unknown",
	KindDiskImage:      "disk-image",
	KindContainerImage: "container-image",
	KindMemoryState:    "memory-state",
	KindOfficeDoc:      "office-doc",
	KindImage:          "image",
	KindAudio:          "audio",
	KindVideo:          "video",
	KindText:           "text",
	KindSource:         "source",
	KindMarkup:         "markup",
	KindOther:          "other",
}

// String returns the Kind's canonical name; unknown values read "unknown".
func (k Kind) String() string {
	if int(k) >= 0 && int(k) < len(kindNames) && kindNames[k] != "" {
		return kindNames[k]
	}
	return kindNames[KindUnknown]
}

// KindValues returns the canonical enum order, from which the Debug tab renders
// the vocabulary and the tests derive the set of values that must have a meaning.
func KindValues() []Kind {
	return []Kind{
		KindUnknown, KindDiskImage, KindContainerImage, KindMemoryState,
		KindOfficeDoc, KindImage, KindAudio, KindVideo,
		KindText, KindSource, KindMarkup, KindOther,
	}
}

// typeKind maps each specific Type id to its broad Kind. It is the single source
// of the Type->Kind relation; the scan package's extension table produces Type
// ids that must all appear here (guarded by a scan test).
var typeKind = map[string]Kind{
	// Disk images.
	"vdi": KindDiskImage, "vmdk": KindDiskImage, "vhd": KindDiskImage,
	"ova": KindDiskImage, "ovf": KindDiskImage, "vbox": KindDiskImage,
	"vzv": KindDiskImage, "vzvhd": KindDiskImage, "ploop": KindDiskImage,
	"xva": KindDiskImage, "hdd": KindDiskImage, "hdds": KindDiskImage,
	"pvm": KindDiskImage, "qcow2": KindDiskImage, "raw": KindDiskImage,
	"vhdx": KindDiskImage,
	// Container images.
	"tar": KindContainerImage, "oci": KindContainerImage, "iso": KindContainerImage,
	// Memory / state saves.
	"vmem": KindMemoryState, "vmss": KindMemoryState, "vmsn": KindMemoryState,
	"dmp": KindMemoryState, "mem": KindMemoryState,
	// Office / documents.
	"doc": KindOfficeDoc, "docx": KindOfficeDoc, "xls": KindOfficeDoc,
	"xlsx": KindOfficeDoc, "ppt": KindOfficeDoc, "pptx": KindOfficeDoc,
	"odt": KindOfficeDoc, "ods": KindOfficeDoc, "odp": KindOfficeDoc,
	"pdf": KindOfficeDoc, "rtf": KindOfficeDoc, "epub": KindOfficeDoc,
	// Images.
	"jpg": KindImage, "png": KindImage, "gif": KindImage,
	"webp": KindImage, "bmp": KindImage, "tiff": KindImage,
	"ico": KindImage, "heic": KindImage,
	// Audio.
	"mp3": KindAudio, "wav": KindAudio, "ogg": KindAudio, "flac": KindAudio,
	"m4a": KindAudio, "aac": KindAudio, "wma": KindAudio, "opus": KindAudio,
	"midi": KindAudio,
	// Video.
	"mp4": KindVideo, "mkv": KindVideo, "avi": KindVideo, "mov": KindVideo,
	"webm": KindVideo, "wmv": KindVideo, "flv": KindVideo, "m4v": KindVideo,
	"mpeg": KindVideo,
	// Text.
	"txt": KindText, "log": KindText, "csv": KindText, "tsv": KindText,
	"ini": KindText, "cfg": KindText, "conf": KindText,
	// Programming source.
	"go": KindSource, "py": KindSource, "js": KindSource, "ts": KindSource,
	"jsx": KindSource, "tsx": KindSource, "rs": KindSource, "c": KindSource,
	"cpp": KindSource, "cc": KindSource, "cxx": KindSource, "h": KindSource,
	"hpp": KindSource, "java": KindSource, "kt": KindSource, "swift": KindSource,
	"rb": KindSource, "php": KindSource, "cs": KindSource, "sh": KindSource,
	"bash": KindSource, "zsh": KindSource, "fish": KindSource, "ps1": KindSource,
	"lua": KindSource, "r": KindSource, "dart": KindSource, "scala": KindSource,
	"sql": KindSource,
	// Markup / structured text.
	"html": KindMarkup, "css": KindMarkup, "xml": KindMarkup,
	"yaml": KindMarkup, "yml": KindMarkup, "json": KindMarkup, "toml": KindMarkup,
	"md": KindMarkup, "rst": KindMarkup, "tex": KindMarkup,
}

// KindForType maps a specific Type id to its broad Kind. An empty or unknown
// Type id yields KindUnknown; the scan package assigns KindOther to a real file
// whose extension matched nothing.
func KindForType(typeID string) Kind {
	if k, ok := typeKind[typeID]; ok {
		return k
	}
	return KindUnknown
}

// KindOf returns a node's broad Kind. It prefers the Kind field the scanner set
// and falls back to deriving it from Type when the field is unset (a hand-built
// node, or an export without a numeric kind). It is the single resolver shared
// by the UI, filter and export layers so they cannot drift apart.
func KindOf(n *Node) Kind {
	if n == nil {
		return KindUnknown
	}
	if n.Kind != KindUnknown {
		return n.Kind
	}
	return KindForType(n.Type)
}
