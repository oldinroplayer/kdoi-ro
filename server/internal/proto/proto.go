// Package proto defines the binary packets exchanged over the WebSocket.
//
// Every WebSocket binary message carries exactly one packet: a little-endian
// uint16 opcode followed by the packet's fields. Strings are a uint8 byte
// length followed by UTF-8 bytes. See server/README.md for the packet table.
package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Client -> server opcodes.
const (
	OpLogin    uint16 = 0x0001
	OpMove     uint16 = 0x0002
	OpNpcClick uint16 = 0x0003
	OpShopBuy  uint16 = 0x0004
	OpEquip    uint16 = 0x0005
	OpUnequip  uint16 = 0x0006
	OpStatusUp uint16 = 0x0007
	OpAttack   uint16 = 0x0008
	OpUseItem  uint16 = 0x0009
	OpRespawn  uint16 = 0x000A
	OpShopSell uint16 = 0x000B
)

// Server -> client opcodes.
const (
	OpLoginResult     uint16 = 0x8001
	OpEnterMap        uint16 = 0x8002
	OpPlayerAppear    uint16 = 0x8003
	OpPlayerDisappear uint16 = 0x8004
	OpPlayerMove      uint16 = 0x8005
	OpPlayerStop      uint16 = 0x8006
	OpWarpPortal      uint16 = 0x8007
	OpNpcAppear       uint16 = 0x8008
	OpShopOpen        uint16 = 0x8009
	OpShopResult      uint16 = 0x800A
	OpInventory       uint16 = 0x800B
	OpStatus          uint16 = 0x800C
	OpMessage         uint16 = 0x800D
	OpMonsterAppear   uint16 = 0x800E
	OpDamage          uint16 = 0x800F
	OpUnitDead        uint16 = 0x8010
	OpLevelUp         uint16 = 0x8011
	OpHeal            uint16 = 0x8012
	OpPlayerLook      uint16 = 0x8013
)

// ShopResult codes.
const (
	ShopOK         uint8 = 0
	ShopNoZeny     uint8 = 1
	ShopOverweight uint8 = 2
	ShopFull       uint8 = 3
	ShopInvalid    uint8 = 4 // unknown item, too far from the NPC, bad amounts
	ShopSold       uint8 = 5 // a sell order went through
)

// LoginResult codes.
const (
	LoginOK              uint8 = 0 // existing character, password matched
	LoginCreated         uint8 = 1 // new character created
	LoginWrongPassword   uint8 = 2
	LoginInvalidName     uint8 = 3
	LoginInvalidPassword uint8 = 4
	LoginAlreadyOnline   uint8 = 5
	LoginServerError     uint8 = 6
)

var ErrShort = errors.New("proto: packet too short")

// Writer builds a packet.
type Writer struct{ buf []byte }

func NewPacket(op uint16) *Writer {
	w := &Writer{buf: make([]byte, 0, 32)}
	return w.U16(op)
}

func (w *Writer) U8(v uint8) *Writer   { w.buf = append(w.buf, v); return w }
func (w *Writer) U16(v uint16) *Writer { w.buf = binary.LittleEndian.AppendUint16(w.buf, v); return w }
func (w *Writer) U32(v uint32) *Writer { w.buf = binary.LittleEndian.AppendUint32(w.buf, v); return w }

// String writes a length-prefixed string; callers keep strings under 256 bytes.
func (w *Writer) String(s string) *Writer {
	if len(s) > 255 {
		s = s[:255]
	}
	w.buf = append(w.buf, uint8(len(s)))
	w.buf = append(w.buf, s...)
	return w
}

func (w *Writer) Bytes() []byte { return w.buf }

// Reader decodes a packet. The first error sticks; check Err after reading.
type Reader struct {
	b   []byte
	off int
	err error
}

func NewReader(b []byte) *Reader { return &Reader{b: b} }

func (r *Reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if r.off+n > len(r.b) {
		r.err = ErrShort
		return nil
	}
	s := r.b[r.off : r.off+n]
	r.off += n
	return s
}

func (r *Reader) U8() uint8 {
	if s := r.take(1); s != nil {
		return s[0]
	}
	return 0
}

func (r *Reader) U16() uint16 {
	if s := r.take(2); s != nil {
		return binary.LittleEndian.Uint16(s)
	}
	return 0
}

func (r *Reader) U32() uint32 {
	if s := r.take(4); s != nil {
		return binary.LittleEndian.Uint32(s)
	}
	return 0
}

func (r *Reader) String() string {
	n := int(r.U8())
	return string(r.take(n))
}

func (r *Reader) Err() error { return r.err }

// Client packets.

type Login struct{ Name, Password string }

type Move struct{ X, Y uint16 }

type NpcClick struct{ NpcID uint32 }

type ShopLine struct{ ItemID, Amount uint16 }

type ShopBuy struct {
	NpcID uint32
	Lines []ShopLine
}

type Equip struct{ Index uint16 }

type Unequip struct{ Index uint16 }

type StatusUp struct{ Stat uint8 }

type Attack struct{ TargetID uint32 }

type UseItem struct{ Index uint16 }

type Respawn struct{}

type SellLine struct{ Index, Amount uint16 }

type ShopSell struct {
	NpcID uint32
	Lines []SellLine
}

// maxShopLines bounds a ShopBuy so a client can't make us allocate much.
const maxShopLines = 50

// Decode reads one client packet and returns a pointer to its struct.
func Decode(b []byte) (any, error) {
	r := NewReader(b)
	op := r.U16()
	var pkt any
	switch op {
	case OpLogin:
		pkt = &Login{Name: r.String(), Password: r.String()}
	case OpMove:
		pkt = &Move{X: r.U16(), Y: r.U16()}
	case OpNpcClick:
		pkt = &NpcClick{NpcID: r.U32()}
	case OpShopBuy:
		b := &ShopBuy{NpcID: r.U32()}
		n := int(r.U16())
		if n > maxShopLines {
			return nil, fmt.Errorf("proto: shop order with %d lines", n)
		}
		for range n {
			b.Lines = append(b.Lines, ShopLine{ItemID: r.U16(), Amount: r.U16()})
		}
		pkt = b
	case OpEquip:
		pkt = &Equip{Index: r.U16()}
	case OpUnequip:
		pkt = &Unequip{Index: r.U16()}
	case OpStatusUp:
		pkt = &StatusUp{Stat: r.U8()}
	case OpAttack:
		pkt = &Attack{TargetID: r.U32()}
	case OpUseItem:
		pkt = &UseItem{Index: r.U16()}
	case OpRespawn:
		pkt = &Respawn{}
	case OpShopSell:
		b := &ShopSell{NpcID: r.U32()}
		n := int(r.U16())
		if n > maxShopLines {
			return nil, fmt.Errorf("proto: sell order with %d lines", n)
		}
		for range n {
			b.Lines = append(b.Lines, SellLine{Index: r.U16(), Amount: r.U16()})
		}
		pkt = b
	default:
		if r.Err() != nil {
			return nil, r.Err()
		}
		return nil, fmt.Errorf("proto: unknown opcode %#04x", op)
	}
	if r.Err() != nil {
		return nil, r.Err()
	}
	return pkt, nil
}

// Server packets.

func LoginResult(code uint8) []byte {
	return NewPacket(OpLoginResult).U8(code).Bytes()
}

// EnterMap tells the client which map to load and where its own character is.
func EnterMap(selfID uint32, mapName string, x, y int, dir uint8, job uint16, speed uint16, look Look) []byte {
	w := NewPacket(OpEnterMap).U32(selfID).String(mapName).
		U16(uint16(x)).U16(uint16(y)).U8(dir).U16(job).U16(speed)
	return look.write(w).Bytes()
}

// Look is a player's visible equipment as item IDs (0 = nothing):
// weapon, shield, upper/middle/lower headgear.
type Look [5]int

func (l Look) write(w *Writer) *Writer {
	for _, id := range l {
		w.U16(uint16(id))
	}
	return w
}

// PlayerAppear shows a player; dead players lie on the ground.
func PlayerAppear(id uint32, name string, job uint16, x, y int, dir uint8, speed uint16, dead bool, look Look) []byte {
	w := NewPacket(OpPlayerAppear).U32(id).String(name).U16(job).
		U16(uint16(x)).U16(uint16(y)).U8(dir).U16(speed).U8(boolU8(dead))
	return look.write(w).Bytes()
}

// PlayerLook updates a player's visible equipment after it changes.
func PlayerLook(id uint32, look Look) []byte {
	return look.write(NewPacket(OpPlayerLook).U32(id)).Bytes()
}

// Disappear reasons.
const (
	GoneOut  uint8 = 0 // left the map or logged out
	GoneDied uint8 = 1 // a monster was killed
)

// PlayerDisappear removes any unit (player or monster) from the client.
func PlayerDisappear(id uint32, reason uint8) []byte {
	return NewPacket(OpPlayerDisappear).U32(id).U8(reason).Bytes()
}

// PlayerMove announces a walk starting at (fromX, fromY) through each cell in
// steps, one cell per `speed` ms (diagonal steps take 1.4x as long).
func PlayerMove(id uint32, fromX, fromY int, steps [][2]int) []byte {
	w := NewPacket(OpPlayerMove).U32(id).U16(uint16(fromX)).U16(uint16(fromY)).U16(uint16(len(steps)))
	for _, s := range steps {
		w.U16(uint16(s[0])).U16(uint16(s[1]))
	}
	return w.Bytes()
}

// WarpPortal marks a warp on the current map (sent after EnterMap).
func WarpPortal(x, y int) []byte {
	return NewPacket(OpWarpPortal).U16(uint16(x)).U16(uint16(y)).Bytes()
}

// PlayerStop snaps a player to a cell (end of walk or correction).
func PlayerStop(id uint32, x, y int) []byte {
	return NewPacket(OpPlayerStop).U32(id).U16(uint16(x)).U16(uint16(y)).Bytes()
}

// NpcAppear shows an NPC; sprite is the file under data/sprite/npc/ without extension.
func NpcAppear(id uint32, name, sprite string, x, y int, dir uint8) []byte {
	return NewPacket(OpNpcAppear).U32(id).String(name).String(sprite).
		U16(uint16(x)).U16(uint16(y)).U8(dir).Bytes()
}

// ShopItem is one entry of a shop's catalogue.
type ShopItem struct {
	ItemID int
	Price  int
}

func ShopOpen(npcID uint32, npcName string, items []ShopItem) []byte {
	w := NewPacket(OpShopOpen).U32(npcID).String(npcName).U16(uint16(len(items)))
	for _, it := range items {
		w.U16(uint16(it.ItemID)).U32(uint32(it.Price))
	}
	return w.Bytes()
}

func ShopResult(code uint8) []byte {
	return NewPacket(OpShopResult).U8(code).Bytes()
}

// InvEntry is one inventory entry as sent to the client.
type InvEntry struct {
	Index, ItemID, Amount int
	Equip                 uint16
}

// Inventory replaces the client's whole inventory.
func Inventory(items []InvEntry) []byte {
	w := NewPacket(OpInventory).U16(uint16(len(items)))
	for _, it := range items {
		w.U16(uint16(it.Index)).U16(uint16(it.ItemID)).U16(uint16(it.Amount)).U16(it.Equip)
	}
	return w.Bytes()
}

// StatusInfo is the character status window's contents.
type StatusInfo struct {
	BaseLevel, JobLevel  int
	Stats, RaiseCosts    [6]int
	StatusPoints, Zeny   int
	HP, MaxHP, SP, MaxSP int
	Atk, Atk2            int
	MatkMin, MatkMax     int
	Def, Def2            int
	Mdef, Mdef2          int
	Hit, Flee, Flee2     int
	Crit, Aspd           int
	Weight, MaxWeight    int
	BaseExp, BaseNext    int // Next is 0 at the level cap
	JobExp, JobNext      int
}

func Status(s StatusInfo) []byte {
	w := NewPacket(OpStatus).U16(uint16(s.BaseLevel)).U16(uint16(s.JobLevel))
	for _, v := range s.Stats {
		w.U8(uint8(v))
	}
	for _, v := range s.RaiseCosts {
		w.U8(uint8(v))
	}
	w.U16(uint16(s.StatusPoints)).U32(uint32(s.Zeny))
	for _, v := range []int{s.HP, s.MaxHP, s.SP, s.MaxSP, s.Atk, s.Atk2, s.MatkMin, s.MatkMax,
		s.Def, s.Def2, s.Mdef, s.Mdef2, s.Hit, s.Flee, s.Flee2, s.Crit, s.Aspd} {
		w.U16(uint16(v))
	}
	return w.U32(uint32(s.Weight)).U32(uint32(s.MaxWeight)).
		U32(uint32(s.BaseExp)).U32(uint32(s.BaseNext)).U32(uint32(s.JobExp)).U32(uint32(s.JobNext)).Bytes()
}

// Message is a short notice for the player (e.g. why an action failed).
func Message(text string) []byte {
	return NewPacket(OpMessage).String(text).Bytes()
}

func boolU8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// MonsterAppear shows a monster; sprite is under data/sprite/몬스터/.
func MonsterAppear(id uint32, mobID int, name, sprite string, x, y int, dir uint8, speed int) []byte {
	return NewPacket(OpMonsterAppear).U32(id).U16(uint16(mobID)).String(name).String(sprite).
		U16(uint16(x)).U16(uint16(y)).U8(dir).U16(uint16(speed)).Bytes()
}

// Damage flags.
const (
	DamageMiss uint8 = 1
	DamageCrit uint8 = 2
)

// Damage shows src attacking target. motion is the attack animation length
// in ms; hurt is how long the target flinches (0 = no flinch).
func Damage(src, target uint32, damage int, flags uint8, motion, hurt int) []byte {
	return NewPacket(OpDamage).U32(src).U32(target).U32(uint32(damage)).U8(flags).
		U16(uint16(motion)).U16(uint16(hurt)).Bytes()
}

// UnitDead shows a player falling (monsters vanish with PlayerDisappear GoneDied).
func UnitDead(id uint32) []byte {
	return NewPacket(OpUnitDead).U32(id).Bytes()
}

// Level-up kinds.
const (
	LevelBase uint8 = 0
	LevelJob  uint8 = 1
)

func LevelUp(id uint32, kind uint8) []byte {
	return NewPacket(OpLevelUp).U32(id).U8(kind).Bytes()
}

// Heal shows HP restored on a unit (e.g. from a potion).
func Heal(id uint32, amount int) []byte {
	return NewPacket(OpHeal).U32(id).U16(uint16(amount)).Bytes()
}
