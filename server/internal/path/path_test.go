package path

import (
	"strings"
	"testing"
)

type grid []string // '#' is blocked; row 0 is y = 0

func (g grid) Walkable(x, y int) bool {
	return y >= 0 && y < len(g) && x >= 0 && x < len(g[y]) && g[y][x] != '#'
}

func TestStraightAndDiagonal(t *testing.T) {
	g := grid{".....", ".....", "....."}
	p := Find(g, Point{0, 0}, Point{2, 2}, 32)
	if len(p) != 2 || p[1] != (Point{2, 2}) {
		t.Fatalf("diagonal path = %v", p)
	}
	p = Find(g, Point{0, 0}, Point{4, 0}, 32)
	if len(p) != 4 || p[3] != (Point{4, 0}) {
		t.Fatalf("straight path = %v", p)
	}
}

func TestRoutesAroundWall(t *testing.T) {
	g := grid{
		".....",
		".###.",
		".....",
	}
	p := Find(g, Point{2, 0}, Point{2, 2}, 32)
	if p == nil || p[len(p)-1] != (Point{2, 2}) {
		t.Fatalf("path = %v", p)
	}
	for i, c := range p {
		if !g.Walkable(c.X, c.Y) {
			t.Fatalf("step %d %v is blocked", i, c)
		}
	}
}

func TestNoCornerCutting(t *testing.T) {
	g := grid{
		".#",
		"..",
	}
	// (1,0) is blocked, so (0,0) -> (1,1) must go via (0,1) instead of cutting the corner.
	p := Find(g, Point{0, 0}, Point{1, 1}, 32)
	if len(p) != 2 {
		t.Fatalf("expected a 2-step detour, got %v", p)
	}
}

func TestUnreachableAndLimits(t *testing.T) {
	g := grid{"..#..", "..#..", "..#.."}
	if p := Find(g, Point{0, 0}, Point{4, 0}, 32); p != nil {
		t.Fatalf("expected nil, got %v", p)
	}
	if p := Find(g, Point{0, 0}, Point{2, 0}, 32); p != nil {
		t.Fatal("blocked target should be nil")
	}
	long := grid{strings.Repeat(".", 50)}
	if p := Find(long, Point{0, 0}, Point{40, 0}, 32); p != nil {
		t.Fatal("path over maxSteps should be nil")
	}
}
