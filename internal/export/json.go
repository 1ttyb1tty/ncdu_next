package export

import (
	"encoding/json"
	"os"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

type Entry struct {
	Name  string  `json:"name"`
	Asize int64   `json:"asize,omitempty"`
	Dsize int64   `json:"dsize,omitempty"`
	Items []Entry `json:"items,omitempty"`
}

func FromNode(n *tree.Node) Entry {
	e := Entry{Name: n.Name, Asize: n.AppSize, Dsize: n.Size}
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
	n := &tree.Node{Name: e.Name, Size: e.Dsize, AppSize: e.Asize}
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
