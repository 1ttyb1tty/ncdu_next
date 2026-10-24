package export

import (
	"encoding/json"
	"os"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

type Entry struct {
	Name       string  `json:"name"`
	Asize      int64   `json:"asize,omitempty"`
	Dsize      int64   `json:"dsize,omitempty"`
	Type       string  `json:"type,omitempty"`
	Kind       uint8   `json:"kind,omitempty"`
	Encoding   string  `json:"encoding,omitempty"`
	LineEnding string  `json:"line_ending,omitempty"`
	Mtime      int64   `json:"mtime,omitempty"`
	Ctime      int64   `json:"ctime,omitempty"`
	Atime      int64   `json:"atime,omitempty"`
	Attrs      uint32  `json:"attrs,omitempty"`
	Items      []Entry `json:"items,omitempty"`
}

func FromNode(n *tree.Node) Entry {
	e := Entry{
		Name:       n.Name,
		Asize:      n.AppSize,
		Dsize:      n.Size,
		Type:       n.Type,
		Kind:       uint8(n.Kind),
		Encoding:   n.Encoding,
		LineEnding: n.LineEnding,
		Mtime:      n.Mtime,
		Ctime:      n.Ctime,
		Atime:      n.Atime,
		Attrs:      n.Attrs,
	}
	if n.IsDir {
		for _, c := range n.Children {
			e.Items = append(e.Items, FromNode(c))
		}
	}
	return e
}

func Read(path string) (*tree.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return ToNode(e), nil
}

func ToNode(e Entry) *tree.Node {
	n := &tree.Node{
		Name:       e.Name,
		Size:       e.Dsize,
		AppSize:    e.Asize,
		Type:       e.Type,
		Kind:       tree.Kind(e.Kind),
		Encoding:   e.Encoding,
		LineEnding: e.LineEnding,
		Mtime:      e.Mtime,
		Ctime:      e.Ctime,
		Atime:      e.Atime,
		Attrs:      e.Attrs,
	}
	// An export without a numeric kind (a partial or hand-authored file) may
	// still carry a specific Type id; KindOf derives the Kind from it. Ids from
	// the retired category vocabulary (e.g. "image-png") do not resolve and stay
	// KindUnknown, which the documented missing-field default allows.
	n.Kind = tree.KindOf(n)
	if len(e.Items) > 0 {
		n.IsDir = true
		for _, c := range e.Items {
			n.AddChild(ToNode(c))
		}
		n.Recompute()
	}
	return n
}
func Write(path string, n *tree.Node) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(FromNode(n))
}
