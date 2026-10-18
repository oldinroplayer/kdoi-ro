package server

import (
	"context"
	"encoding/binary"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kdoiro/server/internal/account"
	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
	"kdoiro/server/internal/proto"
	"kdoiro/server/internal/world"
)

func openGrid(t *testing.T, w, h int) *gat.Map {
	t.Helper()
	data := make([]byte, 14+w*h*20)
	copy(data, "GRAT")
	binary.LittleEndian.PutUint32(data[6:], uint32(w))
	binary.LittleEndian.PutUint32(data[10:], uint32(h))
	m, err := gat.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

const (
	shopNpc  = 50001
	dummyMob = 9001
	bruteMob = 9002
)

// Test-only monsters: neither moves, so they never send unexpected packets.
func init() {
	// Dies to any hit and is worth exactly one base and one job level.
	game.Mobs[dummyMob] = &game.MobDef{
		ID: dummyMob, Name: "Dummy", Sprite: "drops", Level: 1, HP: 1, BaseExp: 9, JobExp: 10,
		Range: 1, ChaseRange: 12, WalkSpeed: 400, AttackDelay: 1000, AttackMotion: 500, DamageMotion: 100,
		Drops: []game.Drop{{ItemID: 501, Rate: 10000}},
	}
	// Hits back hard and fast; can't be killed in a test.
	game.Mobs[bruteMob] = &game.MobDef{
		ID: bruteMob, Name: "Brute", Sprite: "drops", Level: 99, HP: 1_000_000, Atk1: 500, Atk2: 500,
		Stats: [game.NumStats]int{game.DEX: 99}, Range: 1, ChaseRange: 12,
		WalkSpeed: 400, AttackDelay: 100, AttackMotion: 50, DamageMotion: 1, Fights: true,
	}
}

// startServer runs a world + server on a 20x20 open "morocc" and a 10x10 "field",
// with a warp at morocc (15,10) to field (5,5) and back at field (2,2) to morocc (12,10).
func startServer(t *testing.T) (string, *account.Store) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store, err := account.Open(filepath.Join(t.TempDir(), "chars.json"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := world.New(world.Config{
		Maps: map[string]*gat.Map{"morocc": openGrid(t, 20, 20), "field": openGrid(t, 10, 10)},
		Warps: []world.Warp{
			{Map: "morocc", X: 15, Y: 10, DestMap: "field", DestX: 5, DestY: 5},
			{Map: "field", X: 2, Y: 2, DestMap: "morocc", DestX: 12, DestY: 10},
		},
		NPCs: []world.NPC{{ID: shopNpc, Name: "Shop", Sprite: "1_m_merchant", Map: "morocc", X: 10, Y: 12, Shop: []int{1201, 2301, 501}}},
		Spawns: []world.Spawn{
			{Map: "morocc", X: 14, Y: 14, MobID: dummyMob, Count: 1},
			{Map: "morocc", X: 5, Y: 15, MobID: bruteMob, Count: 1},
		},
		SavePoint: world.Location{Map: "morocc", X: 10, Y: 10},
		Seed:      42,
	})
	if err != nil {
		t.Fatal(err)
	}
	go w.Run(ctx)
	srv := New(ctx, store, w, Spawn{Map: "morocc", X: 10, Y: 10}, nil)
	// Cleanups run last-in first-out: this runs before TempDir removal, so
	// sessions finish saving into the temp dir before it's deleted.
	t.Cleanup(func() {
		cancel()
		srv.Wait()
	})
	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)
	return "ws" + strings.TrimPrefix(hs.URL, "http"), store
}

type client struct {
	t    *testing.T
	conn *websocket.Conn
}

func dial(t *testing.T, url string) *client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return &client{t: t, conn: conn}
}

func (c *client) send(b []byte) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageBinary, b); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) login(name, pw string) {
	c.send(proto.NewPacket(proto.OpLogin).String(name).String(pw).Bytes())
}

// background packets that expect skips unless a test asks for them.
var background = map[uint16]bool{
	proto.OpWarpPortal:    true,
	proto.OpNpcAppear:     true,
	proto.OpMonsterAppear: true,
	proto.OpPlayerStop:    true,
	proto.OpInventory:     true,
	proto.OpStatus:        true,
}

// expect reads packets until op arrives (skipping background ones), checks
// there was nothing unexpected in between, and returns a reader past the opcode.
func (c *client) expect(op uint16) *proto.Reader {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			c.t.Fatalf("waiting for %#04x: %v", op, err)
		}
		r := proto.NewReader(data)
		got := r.U16()
		if got == op {
			return r
		}
		if !background[got] {
			c.t.Fatalf("got opcode %#04x, want %#04x", got, op)
		}
	}
}

func (c *client) expectLogin(code uint8) {
	c.t.Helper()
	if got := c.expect(proto.OpLoginResult).U8(); got != code {
		c.t.Fatalf("login result = %d, want %d", got, code)
	}
}

// expectEnterMap checks EnterMap for mapName and returns the self ID and position.
func (c *client) expectEnterMap(mapName string) (id uint32, x, y int) {
	c.t.Helper()
	r := c.expect(proto.OpEnterMap)
	id = r.U32()
	if m := r.String(); m != mapName {
		c.t.Fatalf("map = %q, want %q", m, mapName)
	}
	return id, int(r.U16()), int(r.U16())
}

func (c *client) expectEnter() (id uint32, x, y int) {
	c.t.Helper()
	return c.expectEnterMap("morocc")
}

func TestLoginCreateMoveAndLeave(t *testing.T) {
	url, _ := startServer(t)

	alice := dial(t, url)
	alice.login("Alice", "pass1234")
	alice.expectLogin(proto.LoginCreated)
	aliceID, x, y := alice.expectEnter()
	if x != 10 || y != 10 {
		t.Fatalf("spawn = (%d,%d)", x, y)
	}

	bob := dial(t, url)
	bob.login("ab", "pass1234")
	bob.expectLogin(proto.LoginInvalidName)
	bob.login("Bob1", "pass1234")
	bob.expectLogin(proto.LoginCreated)
	bobID, _, _ := bob.expectEnter()
	if r := bob.expect(proto.OpPlayerAppear); r.U32() != aliceID || r.String() != "Alice" {
		t.Fatal("bob should see alice")
	}
	if r := alice.expect(proto.OpPlayerAppear); r.U32() != bobID || r.String() != "Bob1" {
		t.Fatal("alice should see bob")
	}

	// A second Alice: wrong password, then right password while online.
	imposter := dial(t, url)
	imposter.login("alice", "wrongpass")
	imposter.expectLogin(proto.LoginWrongPassword)
	imposter.login("alice", "pass1234")
	imposter.expectLogin(proto.LoginAlreadyOnline)
	imposter.conn.Close(websocket.StatusNormalClosure, "")

	alice.send(proto.NewPacket(proto.OpMove).U16(12).U16(10).Bytes())
	for _, c := range []*client{alice, bob} {
		r := c.expect(proto.OpPlayerMove)
		if id, fx, fy, n := r.U32(), r.U16(), r.U16(), r.U16(); id != aliceID || fx != 10 || fy != 10 || n != 2 {
			t.Fatalf("move = id %d from (%d,%d) steps %d", id, fx, fy, n)
		}
	}

	time.Sleep(2*world.WalkSpeed*time.Millisecond + 3*world.TickInterval)
	alice.conn.Close(websocket.StatusNormalClosure, "")
	if r := bob.expect(proto.OpPlayerDisappear); r.U32() != aliceID {
		t.Fatal("bob should see alice leave")
	}

	// Position was saved on logout.
	again := dial(t, url)
	again.login("Alice", "pass1234")
	again.expectLogin(proto.LoginOK)
	if _, x, y := again.expectEnter(); x != 12 || y != 10 {
		t.Fatalf("rejoined at (%d,%d), want (12,10)", x, y)
	}
}

func TestWarpBetweenMaps(t *testing.T) {
	url, _ := startServer(t)

	alice := dial(t, url)
	alice.login("Alice", "pass1234")
	alice.expectLogin(proto.LoginCreated)
	aliceID, _, _ := alice.expectEnter()

	bob := dial(t, url)
	bob.login("Bob1", "pass1234")
	bob.expectLogin(proto.LoginCreated)
	bob.expectEnter()
	bob.expect(proto.OpPlayerAppear)   // alice
	alice.expect(proto.OpPlayerAppear) // bob

	// Walk onto the warp at (15,10): bob sees alice leave, alice lands on the field.
	alice.send(proto.NewPacket(proto.OpMove).U16(15).U16(10).Bytes())
	alice.expect(proto.OpPlayerMove)
	bob.expect(proto.OpPlayerMove)
	if r := bob.expect(proto.OpPlayerDisappear); r.U32() != aliceID {
		t.Fatal("bob should see alice warp out")
	}
	if id, x, y := alice.expectEnterMap("field"); id != aliceID || x != 5 || y != 5 {
		t.Fatalf("warped to id %d (%d,%d)", id, x, y)
	}

	// And back: field (2,2) -> morocc (12,10), where bob sees her reappear.
	alice.send(proto.NewPacket(proto.OpMove).U16(2).U16(2).Bytes())
	alice.expect(proto.OpPlayerMove)
	if _, x, y := alice.expectEnter(); x != 12 || y != 10 {
		t.Fatalf("returned to (%d,%d)", x, y)
	}
	alice.expect(proto.OpPlayerAppear) // bob
	if r := bob.expect(proto.OpPlayerAppear); r.U32() != aliceID {
		t.Fatal("bob should see alice return")
	}
}

func TestMoveBeforeLoginIsRejected(t *testing.T) {
	url, _ := startServer(t)
	c := dial(t, url)
	c.send(proto.NewPacket(proto.OpMove).U16(1).U16(1).Bytes())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := c.conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("expected policy violation close, got %v", err)
	}
}

type status struct {
	baseLevel    int
	hp           int
	stats        [6]int
	points, zeny int
	atk2, def    int
	weight       int
}

func (c *client) expectStatus() status {
	c.t.Helper()
	r := c.expect(proto.OpStatus)
	var s status
	s.baseLevel = int(r.U16())
	r.U16() // job level
	for i := range s.stats {
		s.stats[i] = int(r.U8())
	}
	for range 6 {
		r.U8() // raise costs
	}
	s.points, s.zeny = int(r.U16()), int(r.U32())
	v := make([]int, 17) // HP, MaxHP, SP, MaxSP, Atk, Atk2, MatkMin, MatkMax, Def, ...
	for i := range v {
		v[i] = int(r.U16())
	}
	s.hp, s.atk2, s.def = v[0], v[5], v[8]
	s.weight = int(r.U32())
	if r.Err() != nil {
		c.t.Fatal(r.Err())
	}
	return s
}

type invEntry struct {
	index, itemID, amount int
	equip                 uint16
}

func (c *client) expectInventory() map[int]invEntry {
	c.t.Helper()
	r := c.expect(proto.OpInventory)
	byItem := map[int]invEntry{}
	for range int(r.U16()) {
		e := invEntry{int(r.U16()), int(r.U16()), int(r.U16()), r.U16()}
		byItem[e.itemID] = e
	}
	return byItem
}

func TestShopEquipAndStats(t *testing.T) {
	url, _ := startServer(t)
	alice := dial(t, url)
	alice.login("Alice", "pass1234")
	alice.expectLogin(proto.LoginCreated)
	alice.expectEnter()
	if inv := alice.expectInventory(); len(inv) != 0 {
		t.Fatalf("new character has items: %v", inv)
	}
	if s := alice.expectStatus(); s.zeny != 2000 || s.points != 48 || s.stats[0] != 1 {
		t.Fatalf("starting status: %+v", s)
	}

	alice.send(proto.NewPacket(proto.OpNpcClick).U32(shopNpc).Bytes())
	r := alice.expect(proto.OpShopOpen)
	if id, name, n := r.U32(), r.String(), r.U16(); id != shopNpc || name != "Shop" || n != 3 {
		t.Fatalf("shop open: %d %q %d", id, name, n)
	}

	// Knife + Cotton Shirt + 3 Red Potions = 50 + 10 + 150 zeny.
	alice.send(proto.NewPacket(proto.OpShopBuy).U32(shopNpc).U16(3).
		U16(1201).U16(1).U16(2301).U16(1).U16(501).U16(3).Bytes())
	if code := alice.expect(proto.OpShopResult).U8(); code != proto.ShopOK {
		t.Fatalf("buy result %d", code)
	}
	inv := alice.expectInventory()
	if len(inv) != 3 || inv[501].amount != 3 {
		t.Fatalf("inventory after buy: %v", inv)
	}
	if s := alice.expectStatus(); s.zeny != 1790 || s.weight != 40+10+21 {
		t.Fatalf("status after buy: %+v", s)
	}

	alice.send(proto.NewPacket(proto.OpShopBuy).U32(shopNpc).U16(1).U16(501).U16(100).Bytes())
	if code := alice.expect(proto.OpShopResult).U8(); code != proto.ShopNoZeny {
		t.Fatalf("expensive order result %d", code)
	}

	alice.send(proto.NewPacket(proto.OpEquip).U16(uint16(inv[1201].index)).Bytes())
	if r := alice.expect(proto.OpPlayerLook); r.U32() == 0 || r.U16() != 1201 {
		t.Fatal("PlayerLook should show the knife in the weapon slot")
	}
	if inv := alice.expectInventory(); inv[1201].equip != 0x0002 {
		t.Fatalf("knife not equipped: %v", inv)
	}
	if s := alice.expectStatus(); s.atk2 != 17 {
		t.Fatalf("atk2 = %d", s.atk2)
	}
	alice.send(proto.NewPacket(proto.OpEquip).U16(uint16(inv[501].index)).Bytes())
	alice.expect(proto.OpMessage)

	alice.send(proto.NewPacket(proto.OpStatusUp).U8(0).Bytes())
	alice.expectInventory()
	if s := alice.expectStatus(); s.stats[0] != 2 || s.points != 46 {
		t.Fatalf("after STR up: %+v", s)
	}

	// Everything survives logging out and back in.
	alice.conn.Close(websocket.StatusNormalClosure, "")
	time.Sleep(200 * time.Millisecond)
	again := dial(t, url)
	again.login("Alice", "pass1234")
	again.expectLogin(proto.LoginOK)
	again.expectEnter()
	if inv := again.expectInventory(); inv[1201].equip != 0x0002 || inv[501].amount != 3 {
		t.Fatalf("inventory after relogin: %v", inv)
	}
	if s := again.expectStatus(); s.zeny != 1790 || s.stats[0] != 2 || s.atk2 != 17 {
		t.Fatalf("status after relogin: %+v", s)
	}
}

func TestShopTooFar(t *testing.T) {
	url, _ := startServer(t)
	c := dial(t, url)
	c.login("Farther", "pass1234")
	c.expectLogin(proto.LoginCreated)
	c.expectEnter()
	// Walk to (3,3): 9 cells from the NPC at (10,12).
	c.send(proto.NewPacket(proto.OpMove).U16(3).U16(3).Bytes())
	c.expect(proto.OpPlayerMove)
	time.Sleep(8*world.WalkSpeed*14/10*time.Millisecond + 3*world.TickInterval)
	c.send(proto.NewPacket(proto.OpNpcClick).U32(shopNpc).Bytes())
	c.expect(proto.OpMessage)
	c.send(proto.NewPacket(proto.OpShopBuy).U32(shopNpc).U16(1).U16(501).U16(1).Bytes())
	if code := c.expect(proto.OpShopResult).U8(); code != proto.ShopInvalid {
		t.Fatalf("buy from afar: %d", code)
	}
}

// monsterIDs reads past EnterMap and returns the IDs of monsters by mob ID.
func (c *client) loginAndFindMonsters(name string) map[int]uint32 {
	c.t.Helper()
	c.login(name, "pass1234")
	c.expectLogin(proto.LoginCreated)
	c.expectEnter()
	mobs := map[int]uint32{}
	for len(mobs) < 2 {
		r := c.expect(proto.OpMonsterAppear)
		id, mobID := r.U32(), int(r.U16())
		mobs[mobID] = id
	}
	return mobs
}

func TestKillMonsterGivesExpLootAndLevels(t *testing.T) {
	url, _ := startServer(t)
	c := dial(t, url)
	mobs := c.loginAndFindMonsters("Hunter")

	c.send(proto.NewPacket(proto.OpAttack).U32(mobs[dummyMob]).Bytes())
	c.expect(proto.OpPlayerMove) // walking over to (14,14)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var levelUps, items int
	var status status
	killed := false
	for !killed || levelUps < 2 || items == 0 || status.zeny == 0 {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for kill (killed=%v levelUps=%d items=%d): %v", killed, levelUps, items, err)
		}
		r := proto.NewReader(data)
		switch r.U16() {
		case proto.OpPlayerDisappear:
			if r.U32() == mobs[dummyMob] && r.U8() == proto.GoneDied {
				killed = true
			}
		case proto.OpLevelUp:
			levelUps++
		case proto.OpInventory:
			items = int(r.U16())
		case proto.OpStatus:
			if killed && levelUps == 2 {
				// re-parse as a status packet
				status = parseStatus(t, data)
			}
		}
	}
	if items != 1 {
		t.Fatalf("expected the guaranteed Red Potion drop, inventory has %d entries", items)
	}
	if status.baseLevel != 2 || status.points != game.StartPoints+game.StatusPointsFor(2) {
		t.Fatalf("after kill: %+v", status)
	}
}

func TestMonsterKillsPlayerWhoRespawns(t *testing.T) {
	url, _ := startServer(t)
	c := dial(t, url)
	mobs := c.loginAndFindMonsters("Victim")

	c.send(proto.NewPacket(proto.OpAttack).U32(mobs[bruteMob]).Bytes())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var selfID uint32
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			t.Fatalf("waiting to be knocked out: %v", err)
		}
		r := proto.NewReader(data)
		op := r.U16()
		if op == proto.OpDamage {
			if src, target := r.U32(), r.U32(); src == mobs[bruteMob] {
				selfID = target
			}
		}
		if op == proto.OpUnitDead {
			if id := r.U32(); id != selfID {
				t.Fatalf("UnitDead for %d, expected self %d", id, selfID)
			}
			break
		}
	}

	if msg := c.expect(proto.OpMessage).String(); msg != "You have been knocked out." {
		t.Fatalf("death message %q", msg)
	}
	// Dead players can't use items; respawning returns them to the save point.
	c.send(proto.NewPacket(proto.OpUseItem).U16(0).Bytes())
	if msg := c.expect(proto.OpMessage).String(); msg != "You can't do that while knocked out." {
		t.Fatalf("use while dead: %q", msg)
	}
	c.send(proto.NewPacket(proto.OpRespawn).Bytes())
	id, x, y := c.expectEnter()
	if id != selfID || x != 10 || y != 10 {
		t.Fatalf("respawned as %d at (%d,%d)", id, x, y)
	}
	s := c.expectStatus()
	if s.hp != 20 { // half of a level 1 Novice's 40 max HP
		t.Fatalf("respawn HP = %d", s.hp)
	}
}

func parseStatus(t *testing.T, data []byte) status {
	t.Helper()
	r := proto.NewReader(data)
	r.U16()
	var s status
	s.baseLevel = int(r.U16())
	r.U16()
	for i := range s.stats {
		s.stats[i] = int(r.U8())
	}
	for range 6 {
		r.U8()
	}
	s.points, s.zeny = int(r.U16()), int(r.U32())
	s.hp = int(r.U16())
	return s
}
