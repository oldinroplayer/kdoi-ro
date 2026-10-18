// Package gat loads RO .gat files: the per-cell walkability grid of a map.
//
// Layout: "GRAT", major u8, minor u8, width i32, height i32, then width*height
// cells of 20 bytes (4 corner heights f32 + type u32). Cell (x, y) is at index
// y*width + x, with y = 0 at the south edge.
package gat

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

const (
	headerSize = 14
	cellSize   = 20
)

// Cell types used by the client data.
const (
	typeWalkable      = 0
	typeWalkableWater = 3
)

type Map struct {
	Width, Height int
	walkable      []bool
}

func Load(path string) (*Map, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

func Parse(data []byte) (*Map, error) {
	if len(data) < headerSize || string(data[:4]) != "GRAT" {
		return nil, errors.New("not a GAT file")
	}
	w := int(int32(binary.LittleEndian.Uint32(data[6:])))
	h := int(int32(binary.LittleEndian.Uint32(data[10:])))
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil, fmt.Errorf("bad dimensions %dx%d", w, h)
	}
	if len(data) < headerSize+w*h*cellSize {
		return nil, errors.New("truncated cell data")
	}
	m := &Map{Width: w, Height: h, walkable: make([]bool, w*h)}
	for i := range m.walkable {
		t := binary.LittleEndian.Uint32(data[headerSize+i*cellSize+16:])
		m.walkable[i] = t == typeWalkable || t == typeWalkableWater
	}
	return m, nil
}

func (m *Map) Walkable(x, y int) bool {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return false
	}
	return m.walkable[y*m.Width+x]
}

// NearestWalkable searches outward in square rings for the closest walkable cell.
func (m *Map) NearestWalkable(x, y int) (int, int, bool) {
	maxR := max(m.Width, m.Height)
	for r := 0; r <= maxR; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if max(abs(dx), abs(dy)) != r {
					continue
				}
				if m.Walkable(x+dx, y+dy) {
					return x + dx, y + dy, true
				}
			}
		}
	}
	return 0, 0, false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
