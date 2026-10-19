package world

import (
	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
)

// Vendor stalls: NPCs drawn as a sitting Merchant or Blacksmith with a shop
// sign over their head, like players vending in RO. Clicking one opens its
// wares at its own prices (forged elemental weapons among them); they only
// sell, never buy.

// Vendor is what makes an NPC a vending stall.
type Vendor struct {
	Title           string // the sign over their head
	Job             uint16 // game.JobMerchant or the Blacksmith's class
	Female          bool
	Hair, HairColor int
	Stock           []VendItem
}

// VendItem is one ware: an item at a price, a forged weapon if Element is set.
type VendItem struct {
	ItemID  int
	Price   int
	Element int
}

// ware finds a stall's ware by item ID (each item is sold once per stall).
func (v *Vendor) ware(itemID int) (VendItem, bool) {
	for _, it := range v.Stock {
		if it.ItemID == itemID {
			return it, true
		}
	}
	return VendItem{}, false
}

// appearPacket draws the stall: the vendor, sitting, and the sign.
func (n *NPC) vendorAppear() []byte {
	v := n.Vendor
	look := proto.Look{0, 0, 0, 0, 0, v.Hair, v.HairColor, boolInt(v.Female)}
	return proto.VendorAppear(n.ID, n.Name, v.Title, v.Job, n.X, n.Y, n.Dir, look)
}

// openVendor shows a stall's wares: buy only, at its prices.
func (w *World) openVendor(p *Player, n *NPC) {
	items := make([]proto.ShopItem, len(n.Vendor.Stock))
	for i, it := range n.Vendor.Stock {
		items[i] = proto.ShopItem{ItemID: it.ItemID, Price: it.Price, Element: it.Element}
	}
	p.out.Send(proto.ShopOpen(n.ID, n.Vendor.Title, items, 0, true))
}

// stampElements gives the weapons a purchase just added their forged element.
func stampElements(c *game.CharData, v *Vendor, before int) {
	for i := before; i < len(c.Inventory); i++ {
		if it, ok := v.ware(c.Inventory[i].ItemID); ok && it.Element != game.ElemNeutral {
			c.Inventory[i].Element = it.Element
		}
	}
}
