package world

import (
	"encoding/binary"
	"testing"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
	"kdoiro/server/internal/proto"
)

// chatCounter counts the public chat lines a player receives.
type chatCounter struct{ lines int }

func (c *chatCounter) Send(p []byte) {
	if binary.LittleEndian.Uint16(p) == proto.OpChatMessage {
		c.lines++
	}
}

func TestServerChat(t *testing.T) {
	w, err := New(Config{
		Maps:      map[string]*gat.Map{"town": openMap(t, 40, 40), "field": openMap(t, 20, 20)},
		SavePoint: Location{Map: "town", X: 1, Y: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	join := func(id uint32, name, mapName string, x, y int) *chatCounter {
		out := &chatCounter{}
		p := &Player{unit: unit{id: id, speed: WalkSpeed}, Name: name, data: game.NewChar(mapName, x, y), out: out}
		w.players[id] = p
		w.online[lower(name)] = id
		w.place(p, w.maps[mapName], x, y)
		return out
	}
	speaker := join(1, "Ann", "town", 5, 5)
	near := join(2, "Ben", "town", 8, 8)
	far := join(3, "Cid", "town", 35, 35) // beyond chat range
	away := join(4, "Dee", "field", 5, 5) // another map
	say := func() {
		// Chat runs through w.do; run the queued command here as the world loop would.
		go func() { _ = w.Chat(1, "hello") }()
		(<-w.cmds)()
	}

	say()
	if speaker.lines != 1 || near.lines != 1 || far.lines != 0 || away.lines != 0 {
		t.Fatalf("local chat: speaker %d near %d far %d away %d", speaker.lines, near.lines, far.lines, away.lines)
	}
	w.serverChat = true
	say()
	if speaker.lines != 2 || near.lines != 2 || far.lines != 1 || away.lines != 1 {
		t.Fatalf("server chat: speaker %d near %d far %d away %d", speaker.lines, near.lines, far.lines, away.lines)
	}
}
