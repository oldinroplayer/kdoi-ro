package world

import (
	"testing"

	"kdoiro/server/internal/game"
)

func TestShopFixedPrice(t *testing.T) {
	p := &Player{data: game.NewChar("town", 1, 1)}
	regular := &NPC{Shop: []int{1101}}
	if got, want := regular.priceFor(p, 1101), p.data.BuyPrice(game.Items[1101].Price); got != want || got <= 1 {
		t.Fatalf("database price: %d, want %d", got, want)
	}
	cheap := &NPC{Shop: []int{1101}, Price: 1}
	if got := cheap.priceFor(p, 1101); got != 1 {
		t.Fatalf("fixed price: %d, want 1", got)
	}
}
