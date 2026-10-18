package gat

import (
	"encoding/binary"
	"os"
	"testing"
)

// build makes a GAT file; blocked lists cells with type 1.
func build(w, h int, blocked ...[2]int) []byte {
	data := make([]byte, headerSize+w*h*cellSize)
	copy(data, "GRAT")
	data[4], data[5] = 1, 2
	binary.LittleEndian.PutUint32(data[6:], uint32(w))
	binary.LittleEndian.PutUint32(data[10:], uint32(h))
	for _, c := range blocked {
		binary.LittleEndian.PutUint32(data[headerSize+(c[1]*w+c[0])*cellSize+16:], 1)
	}
	return data
}

func TestParse(t *testing.T) {
	m, err := Parse(build(4, 3, [2]int{1, 2}))
	if err != nil {
		t.Fatal(err)
	}
	if m.Width != 4 || m.Height != 3 {
		t.Fatalf("size = %dx%d", m.Width, m.Height)
	}
	if !m.Walkable(0, 0) || m.Walkable(1, 2) || m.Walkable(-1, 0) || m.Walkable(4, 0) {
		t.Fatal("walkability wrong")
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	for name, data := range map[string][]byte{
		"magic":     []byte("NOPE0000000000"),
		"truncated": build(4, 4)[:headerSize+10],
	} {
		if _, err := Parse(data); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestNearestWalkable(t *testing.T) {
	m, _ := Parse(build(5, 5, [2]int{2, 2}, [2]int{1, 2}))
	x, y, ok := m.NearestWalkable(2, 2)
	if !ok || !m.Walkable(x, y) || max(abs(x-2), abs(y-2)) != 1 {
		t.Fatalf("got (%d,%d) ok=%v", x, y, ok)
	}
}

func TestLoadMorocc(t *testing.T) {
	path := "../../../ro-client/data/morocc.gat"
	if _, err := os.Stat(path); err != nil {
		t.Skip("extracted client data not present")
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Width != 320 || m.Height != 320 {
		t.Fatalf("size = %dx%d", m.Width, m.Height)
	}
	if !m.Walkable(156, 93) {
		t.Fatal("spawn cell not walkable")
	}
}
