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
	OpLogin         uint16 = 0x0001
	OpMove          uint16 = 0x0002
	OpNpcClick      uint16 = 0x0003
	OpShopBuy       uint16 = 0x0004
	OpEquip         uint16 = 0x0005
	OpUnequip       uint16 = 0x0006
	OpStatusUp      uint16 = 0x0007
	OpAttack        uint16 = 0x0008
	OpUseItem       uint16 = 0x0009
	OpRespawn       uint16 = 0x000A
	OpShopSell      uint16 = 0x000B
	OpSit           uint16 = 0x000C
	OpTurn          uint16 = 0x000D
	OpLearn         uint16 = 0x000E
	OpUseSkill      uint16 = 0x000F
	OpNpcReply      uint16 = 0x0010
	OpChat          uint16 = 0x0011
	OpWhisper       uint16 = 0x0012
	OpRoomCreate    uint16 = 0x0013
	OpRoomJoin      uint16 = 0x0014
	OpRoomLeave     uint16 = 0x0015
	OpRoomKick      uint16 = 0x0016
	OpRoomUpdate    uint16 = 0x0017
	OpRoomOwner     uint16 = 0x0018
	OpPickUp        uint16 = 0x0019
	OpDropItem      uint16 = 0x001A
	OpSetAppearance uint16 = 0x001B
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
	OpPlayerSit       uint16 = 0x8014
	OpPlayerTurn      uint16 = 0x8015
	OpSkillList       uint16 = 0x8016
	OpSkillCast       uint16 = 0x8017
	OpSkillUse        uint16 = 0x8018
	OpNpcDialog       uint16 = 0x8019
	OpPlayerJob       uint16 = 0x801A
	OpBuff            uint16 = 0x801B
	OpChatMessage     uint16 = 0x801C
	OpWhisperFrom     uint16 = 0x801D
	OpWhisperResult   uint16 = 0x801E
	OpRoomSign        uint16 = 0x801F
	OpRoomGone        uint16 = 0x8020
	OpRoomEnter       uint16 = 0x8021
	OpRoomMember      uint16 = 0x8022
	OpRoomResult      uint16 = 0x8023
	OpRoomLeft        uint16 = 0x8024
	OpRoomSettings    uint16 = 0x8025
	OpFloorItem       uint16 = 0x8026
	OpFloorItemGone   uint16 = 0x8027
	OpPlayerPickUp    uint16 = 0x8028
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

// LongString writes a string with a uint16 length (for dialog text).
func (w *Writer) LongString(s string) *Writer {
	if len(s) > 0xFFFF {
		s = s[:0xFFFF]
	}
	w.buf = binary.LittleEndian.AppendUint16(w.buf, uint16(len(s)))
	w.buf = append(w.buf, s...)
	return w
}

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

// Sit asks to sit down (true) or stand up (false).
type Sit struct{ Sit bool }

// Turn asks to face a direction (0 N, 1 NW, 2 W, ... 7 NE) without moving.
type Turn struct{ Dir uint8 }

// Learn spends a skill point on a skill.
type Learn struct{ SkillID uint16 }

// NpcReply answers an NPC dialog with the chosen option (0-based), or
// NpcReplyClose to walk away.
type NpcReply struct {
	NpcID  uint32
	Choice uint8
}

const NpcReplyClose uint8 = 0xFF

// UseSkill uses a skill at a level (0 = highest learned) on a target unit
// (ignored for self skills).
type UseSkill struct {
	SkillID, Level uint16
	TargetID       uint32
}

type SellLine struct{ Index, Amount uint16 }

// Chat is a line typed into the chat box (to everyone nearby, or to the chat
// room the player is in).
type Chat struct{ Text string }

type Whisper struct{ To, Text string }

// RoomSettings are a chat room's title, size and privacy.
type RoomSettings struct {
	Title    string
	Limit    uint8
	Public   bool
	Password string
}

type RoomCreate struct{ RoomSettings }

type RoomJoin struct {
	RoomID   uint32
	Password string
}

type RoomLeave struct{}

type RoomKick struct{ Name string }

type RoomUpdate struct{ RoomSettings }

type RoomOwner struct{ Name string }

// PickUp picks up an item lying on the ground (walking over first).
type PickUp struct{ ItemID uint32 }

// DropItem puts inventory items on the ground at the player's feet.
type DropItem struct{ Index, Amount uint16 }

// SetAppearance changes the player's hairstyle, hair colour and gender.
type SetAppearance struct {
	Style, Color uint8
	Female       bool
}

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
	case OpSit:
		pkt = &Sit{Sit: r.U8() != 0}
	case OpTurn:
		pkt = &Turn{Dir: r.U8()}
	case OpNpcReply:
		pkt = &NpcReply{NpcID: r.U32(), Choice: r.U8()}
	case OpLearn:
		pkt = &Learn{SkillID: r.U16()}
	case OpUseSkill:
		pkt = &UseSkill{SkillID: r.U16(), Level: r.U16(), TargetID: r.U32()}
	case OpChat:
		pkt = &Chat{Text: r.String()}
	case OpWhisper:
		pkt = &Whisper{To: r.String(), Text: r.String()}
	case OpRoomCreate:
		pkt = &RoomCreate{readRoomSettings(r)}
	case OpRoomUpdate:
		pkt = &RoomUpdate{readRoomSettings(r)}
	case OpRoomJoin:
		pkt = &RoomJoin{RoomID: r.U32(), Password: r.String()}
	case OpRoomLeave:
		pkt = &RoomLeave{}
	case OpRoomKick:
		pkt = &RoomKick{Name: r.String()}
	case OpPickUp:
		pkt = &PickUp{ItemID: r.U32()}
	case OpSetAppearance:
		pkt = &SetAppearance{Style: r.U8(), Color: r.U8(), Female: r.U8() != 0}
	case OpDropItem:
		pkt = &DropItem{Index: r.U16(), Amount: r.U16()}
	case OpRoomOwner:
		pkt = &RoomOwner{Name: r.String()}
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

func readRoomSettings(r *Reader) RoomSettings {
	return RoomSettings{Title: r.String(), Limit: r.U8(), Public: r.U8() != 0, Password: r.String()}
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

// Look is what shows on a player's sprite: weapon, shield, upper/middle/lower
// headgear as item IDs (0 = nothing), then hairstyle (0 = not chosen yet,
// drawn as the default), hair colour and gender (1 = female).
type Look [8]int

func (l Look) write(w *Writer) *Writer {
	for _, id := range l {
		w.U16(uint16(id))
	}
	return w
}

// PlayerAppear shows a player; dead players lie on the ground.
func PlayerAppear(id uint32, name string, job uint16, x, y int, dir uint8, speed uint16, dead bool, look Look, sitting bool) []byte {
	w := NewPacket(OpPlayerAppear).U32(id).String(name).U16(job).
		U16(uint16(x)).U16(uint16(y)).U8(dir).U16(speed).U8(boolU8(dead))
	return look.write(w).U8(boolU8(sitting)).Bytes()
}

// SkillList sends the character's skill tree: skill points left, then every
// skill in its tree with the learned level (0 = not learned yet).
func SkillList(points int, skills [][2]int) []byte {
	w := NewPacket(OpSkillList).U16(uint16(points)).U16(uint16(len(skills)))
	for _, s := range skills {
		w.U16(uint16(s[0])).U8(uint8(s[1]))
	}
	return w.Bytes()
}

// SkillCast shows src starting to cast a skill on target for castMs.
func SkillCast(src, target uint32, skillID, castMs int) []byte {
	return NewPacket(OpSkillCast).U32(src).U32(target).U16(uint16(skillID)).U16(uint16(castMs)).Bytes()
}

// SkillUse shows src using a skill on target (its effect follows separately,
// e.g. as Heal or Damage).
func SkillUse(src, target uint32, skillID, level int) []byte {
	return NewPacket(OpSkillUse).U32(src).U32(target).U16(uint16(skillID)).U8(uint8(level)).Bytes()
}

// PlayerTurn shows a player facing a new direction.
func PlayerTurn(id uint32, dir uint8) []byte {
	return NewPacket(OpPlayerTurn).U32(id).U8(dir).Bytes()
}

// PlayerSit shows a player sitting down (true) or standing up (false).
func PlayerSit(id uint32, sitting bool) []byte {
	return NewPacket(OpPlayerSit).U32(id).U8(boolU8(sitting)).Bytes()
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

// ShopOpen lists what an NPC sells (prices after Discount). sellRate is the
// percentage of half price the shop pays when buying (100, or more with Overcharge).
func ShopOpen(npcID uint32, npcName string, items []ShopItem, sellRate int) []byte {
	w := NewPacket(OpShopOpen).U32(npcID).String(npcName).U16(uint16(len(items)))
	for _, it := range items {
		w.U16(uint16(it.ItemID)).U32(uint32(it.Price))
	}
	return w.U16(uint16(sellRate)).Bytes()
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
	Job                  uint16
	Bonus                [6]int // job level stat bonuses
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
	w.U32(uint32(s.Weight)).U32(uint32(s.MaxWeight)).
		U32(uint32(s.BaseExp)).U32(uint32(s.BaseNext)).U32(uint32(s.JobExp)).U32(uint32(s.JobNext))
	w.U16(s.Job)
	for _, v := range s.Bonus {
		w.U8(uint8(v))
	}
	return w.Bytes()
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

// Damage shows src hitting target. motion is the attack animation length in
// ms; hurt is how long the target flinches (0 = no flinch). skillID is 0 for
// a normal attack; hits is how many blows the damage is split over (bolts).
func Damage(src, target uint32, damage int, flags uint8, motion, hurt, skillID, hits int) []byte {
	return NewPacket(OpDamage).U32(src).U32(target).U32(uint32(damage)).U8(flags).
		U16(uint16(motion)).U16(uint16(hurt)).U16(uint16(skillID)).U8(uint8(max(hits, 1))).Bytes()
}

// BuffIcon shows a buff starting (ms > 0) or ending (ms = 0) on a unit.
func BuffIcon(id uint32, skillID, level, ms int) []byte {
	return NewPacket(OpBuff).U32(id).U16(uint16(skillID)).U8(uint8(level)).U32(uint32(ms)).Bytes()
}

// UnitDead shows a player falling (monsters vanish with PlayerDisappear GoneDied).
func UnitDead(id uint32) []byte {
	return NewPacket(OpUnitDead).U32(id).Bytes()
}

// Level-up kinds.
const (
	LevelBase uint8 = 0
	LevelJob  uint8 = 1
	JobChange uint8 = 2 // changed into a new job
)

func LevelUp(id uint32, kind uint8) []byte {
	return NewPacket(OpLevelUp).U32(id).U8(kind).Bytes()
}

// Heal shows HP restored on a unit (e.g. from a potion).
func Heal(id uint32, amount int) []byte {
	return NewPacket(OpHeal).U32(id).U16(uint16(amount)).Bytes()
}

// NpcDialog shows an NPC's message with reply options (none = just a Close
// button). Text may contain newlines.
func NpcDialog(npcID uint32, name, text string, options []string) []byte {
	w := NewPacket(OpNpcDialog).U32(npcID).String(name).LongString(text).U8(uint8(len(options)))
	for _, o := range options {
		w.String(o)
	}
	return w.Bytes()
}

// PlayerJob shows a player's new job (its sprite changes).
func PlayerJob(id uint32, job uint16) []byte {
	return NewPacket(OpPlayerJob).U32(id).U16(job).Bytes()
}

// Chat message kinds.
const (
	ChatPublic uint8 = 0 // said out loud to everyone nearby
	ChatRoom   uint8 = 1 // said inside a chat room
)

// ChatMessage shows a line someone said. id is the speaker (for the speech
// bubble over their head).
func ChatMessage(id uint32, kind uint8, name, text string) []byte {
	return NewPacket(OpChatMessage).U32(id).U8(kind).String(name).String(text).Bytes()
}

// WhisperFrom delivers a private message.
func WhisperFrom(from, text string) []byte {
	return NewPacket(OpWhisperFrom).String(from).String(text).Bytes()
}

// Whisper results.
const (
	WhisperSent    uint8 = 0
	WhisperOffline uint8 = 1
	WhisperSelf    uint8 = 2
)

func WhisperResult(code uint8, to string) []byte {
	return NewPacket(OpWhisperResult).U8(code).String(to).Bytes()
}

// RoomSign shows (or updates) the sign over a chat room owner's head.
func RoomSign(roomID, ownerID uint32, title string, members, limit int, public bool) []byte {
	return NewPacket(OpRoomSign).U32(roomID).U32(ownerID).String(title).U8(uint8(members)).U8(uint8(limit)).U8(boolU8(public)).Bytes()
}

// RoomGone removes a chat room's sign.
func RoomGone(roomID uint32) []byte { return NewPacket(OpRoomGone).U32(roomID).Bytes() }

// RoomEnter opens the chat room window for a player who created or joined it.
func RoomEnter(roomID uint32, title string, limit int, public bool, owner string, members []string) []byte {
	w := NewPacket(OpRoomEnter).U32(roomID).String(title).U8(uint8(limit)).U8(boolU8(public)).String(owner).U8(uint8(len(members)))
	for _, m := range members {
		w.String(m)
	}
	return w.Bytes()
}

// Room member changes.
const (
	MemberLeft   uint8 = 0
	MemberJoined uint8 = 1
	MemberKicked uint8 = 2
)

func RoomMember(change uint8, name string) []byte {
	return NewPacket(OpRoomMember).U8(change).String(name).Bytes()
}

// Room join/create results.
const (
	RoomFull          uint8 = 1
	RoomWrongPassword uint8 = 2
	RoomBanned        uint8 = 3
	RoomNotFound      uint8 = 4
	RoomAlreadyIn     uint8 = 5
	RoomCantNow       uint8 = 6
	RoomBadSettings   uint8 = 7
)

func RoomResult(code uint8) []byte { return NewPacket(OpRoomResult).U8(code).Bytes() }

// Reasons for leaving a room.
const (
	LeftByChoice uint8 = 0
	LeftKicked   uint8 = 1
)

func RoomLeft(reason uint8) []byte { return NewPacket(OpRoomLeft).U8(reason).Bytes() }

// RoomSettingsChanged tells members the room's new settings and owner.
func RoomSettingsChanged(title string, limit int, public bool, owner string) []byte {
	return NewPacket(OpRoomSettings).String(title).U8(uint8(limit)).U8(boolU8(public)).String(owner).Bytes()
}

// FloorItem shows an item lying on the ground. id is the floor item's unit ID;
// subX/subY place it within the cell (0-15, 8 = centre). dropped means it just
// fell (play the drop animation) rather than was already there.
func FloorItem(id uint32, itemID, amount, x, y int, subX, subY uint8, dropped bool) []byte {
	return NewPacket(OpFloorItem).U32(id).U16(uint16(itemID)).U16(uint16(amount)).
		U16(uint16(x)).U16(uint16(y)).U8(subX).U8(subY).U8(boolU8(dropped)).Bytes()
}

// FloorItemGone removes a floor item (picked up or expired).
func FloorItemGone(id uint32) []byte { return NewPacket(OpFloorItemGone).U32(id).Bytes() }

// PlayerPickUp plays a player's pick-up motion.
func PlayerPickUp(id uint32) []byte { return NewPacket(OpPlayerPickUp).U32(id).Bytes() }
