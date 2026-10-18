// Package path finds 8-directional walking paths on a cell grid with A*.
package path

import "container/heap"

type Point struct{ X, Y int }

type Grid interface {
	Walkable(x, y int) bool
}

const (
	costStraight = 10
	costDiagonal = 14
	// Upper bound on expanded nodes so a click on an unreachable cell
	// can't stall the game loop.
	maxExpanded = 20000
)

var dirs = [8]Point{{0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}, {1, 0}, {1, 1}}

// Find returns the cells from `from` (exclusive) to `to` (inclusive), or nil
// if `to` is unreachable or the path would be longer than maxSteps.
// Diagonal moves may not cut the corner of a blocked cell.
func Find(g Grid, from, to Point, maxSteps int) []Point {
	if from == to || !g.Walkable(to.X, to.Y) {
		return nil
	}
	if octile(from, to)/costStraight > maxSteps {
		return nil
	}

	type nodeInfo struct {
		g, steps int
		parent   Point
		closed   bool
	}
	info := map[Point]*nodeInfo{from: {}}
	open := &pq{}
	heap.Push(open, item{p: from, f: octile(from, to)})

	for expanded := 0; open.Len() > 0 && expanded < maxExpanded; expanded++ {
		cur := heap.Pop(open).(item).p
		ci := info[cur]
		if ci.closed {
			continue
		}
		ci.closed = true
		if cur == to {
			out := make([]Point, ci.steps)
			for p, i := cur, ci.steps-1; i >= 0; p, i = info[p].parent, i-1 {
				out[i] = p
			}
			return out
		}
		if ci.steps >= maxSteps {
			continue
		}
		for _, d := range dirs {
			n := Point{cur.X + d.X, cur.Y + d.Y}
			if !g.Walkable(n.X, n.Y) {
				continue
			}
			cost := costStraight
			if d.X != 0 && d.Y != 0 {
				if !g.Walkable(cur.X+d.X, cur.Y) || !g.Walkable(cur.X, cur.Y+d.Y) {
					continue
				}
				cost = costDiagonal
			}
			ng := ci.g + cost
			ni, seen := info[n]
			if seen && (ni.closed || ni.g <= ng) {
				continue
			}
			info[n] = &nodeInfo{g: ng, steps: ci.steps + 1, parent: cur}
			heap.Push(open, item{p: n, f: ng + octile(n, to)})
		}
	}
	return nil
}

func octile(a, b Point) int {
	dx, dy := abs(a.X-b.X), abs(a.Y-b.Y)
	return costStraight*max(dx, dy) + (costDiagonal-costStraight)*min(dx, dy)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

type item struct {
	p Point
	f int
}

type pq []item

func (q pq) Len() int           { return len(q) }
func (q pq) Less(i, j int) bool { return q[i].f < q[j].f }
func (q pq) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *pq) Push(x any)        { *q = append(*q, x.(item)) }
func (q *pq) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}
