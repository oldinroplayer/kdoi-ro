package server

import (
	"context"
	"testing"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
)

// readUntil reads packets (of any kind) until match returns true.
func (c *client) readUntil(what string, match func(op uint16, r *proto.Reader) bool) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			c.t.Fatalf("waiting for %s: %v", what, err)
		}
		r := proto.NewReader(data)
		if match(r.U16(), r) {
			return
		}
	}
}

type damage struct {
	src, target  uint32
	amount       int
	flags        uint8
	skill, hits  int
	motion, hurt int
}

func readDamage(r *proto.Reader) damage {
	d := damage{src: r.U32(), target: r.U32(), amount: int(r.U32()), flags: r.U8()}
	d.motion, d.hurt = int(r.U16()), int(r.U16())
	d.skill, d.hits = int(r.U16()), int(r.U8())
	return d
}

// loginJob logs in a prepared character and returns its ID and the test
// monsters' unit IDs by mob ID.
func (c *client) loginJob(name string) (uint32, map[int][]uint32) {
	c.t.Helper()
	c.login(name, "pass1234")
	c.expectLogin(proto.LoginOK)
	id, _, _ := c.expectEnter()
	mobs := map[int][]uint32{}
	for range testMonsters {
		r := c.expect(proto.OpMonsterAppear)
		mid, mobID := r.U32(), int(r.U16())
		mobs[mobID] = append(mobs[mobID], mid)
	}
	return id, mobs
}

func useSkill(skill int, target uint32) []byte {
	return proto.NewPacket(proto.OpUseSkill).U16(uint16(skill)).U16(0).U32(target).Bytes()
}

func TestBashWalksIntoRangeAndHits(t *testing.T) {
	url, store := startServer(t)
	createChar(t, store, "Knight", func(c *game.CharData) {
		c.Job, c.BaseLevel, c.SP = game.JobSwordman, 20, 100
		c.Skills[game.SkillBash] = 3
	})
	c := dial(t, url)
	self, mobs := c.loginJob("Knight")
	dummy := mobs[dummyMob][0]

	// The dummy at (14,14) is out of reach: the Swordman walks over first.
	c.send(useSkill(game.SkillBash, dummy))
	c.expect(proto.OpPlayerMove)
	c.readUntil("Bash", func(op uint16, r *proto.Reader) bool {
		if op != proto.OpSkillUse {
			return false
		}
		if src, target, skill, lv := r.U32(), r.U32(), int(r.U16()), int(r.U8()); src != self || target != dummy || skill != game.SkillBash || lv != 3 {
			t.Fatalf("SkillUse = %d %d %d %d", src, target, skill, lv)
		}
		return true
	})
	if d := readDamage(c.expect(proto.OpDamage)); d.src != self || d.target != dummy || d.skill != game.SkillBash || d.hits != 1 {
		t.Fatalf("Bash damage: %+v", d)
	}
}

func TestFireBoltCastsAndHitsOncePerLevel(t *testing.T) {
	url, store := startServer(t)
	createChar(t, store, "Wizard", func(c *game.CharData) {
		c.Job, c.BaseLevel, c.SP = game.JobMage, 30, 500
		c.Stats[game.INT] = 50
		c.Skills[game.SkillFireBolt] = 3
	})
	c := dial(t, url)
	self, mobs := c.loginJob("Wizard")
	dummy := mobs[dummyMob][0]

	c.send(useSkill(game.SkillFireBolt, dummy)) // 4 cells away, in range 9
	r := c.expect(proto.OpSkillCast)
	if src, target, skill, ms := r.U32(), r.U32(), int(r.U16()), int(r.U16()); src != self || target != dummy || skill != game.SkillFireBolt || ms != 2100 {
		t.Fatalf("SkillCast = %d %d %d %d", src, target, skill, ms)
	}
	c.readUntil("the cast to finish", func(op uint16, _ *proto.Reader) bool { return op == proto.OpSkillUse })
	if d := readDamage(c.expect(proto.OpDamage)); d.skill != game.SkillFireBolt || d.hits != 3 || d.amount < 3 || d.flags != 0 {
		t.Fatalf("Fire Bolt damage: %+v", d)
	}
	if r := c.expect(proto.OpPlayerDisappear); r.U32() != dummy || r.U8() != proto.GoneDied {
		t.Fatal("the dummy should die")
	}
}

func TestHealAndIncreaseAgi(t *testing.T) {
	url, store := startServer(t)
	createChar(t, store, "Priest", func(c *game.CharData) {
		c.Job, c.BaseLevel, c.SP, c.HP = game.JobAcolyte, 20, 200, 10
		c.Stats[game.INT] = 20
		c.Skills[game.SkillHeal] = 2
		c.Skills[game.SkillIncAgi] = 1
	})
	c := dial(t, url)
	self, _ := c.loginJob("Priest")

	c.send(useSkill(game.SkillHeal, 0)) // no target: heal yourself
	c.expect(proto.OpSkillUse)
	want := (20 + 20) / 8 * (4 + 8*2)
	if r := c.expect(proto.OpHeal); r.U32() != self || int(r.U16()) != want {
		t.Fatalf("Heal should restore %d", want)
	}

	// Heal's after-cast delay (1 s) blocks the next skill for a moment.
	time.Sleep(1100 * time.Millisecond)
	c.send(useSkill(game.SkillIncAgi, self))
	if r := c.expect(proto.OpSkillCast); r.U32() != self || r.U32() != self || int(r.U16()) != game.SkillIncAgi {
		t.Fatal("expected Increase AGI's cast")
	}
	c.expect(proto.OpSkillUse)
	r := c.expect(proto.OpBuff)
	if id, skill, lv, ms := r.U32(), int(r.U16()), int(r.U8()), int(r.U32()); id != self || skill != game.SkillIncAgi || lv != 1 || ms != 60000 {
		t.Fatalf("Buff = %d %d %d %d", id, skill, lv, ms)
	}
	// Increase AGI also makes walking 25% faster, and the map is told.
	if r := c.expect(proto.OpPlayerSpeed); r.U32() != self || r.U16() != 113 {
		t.Fatal("expected PlayerSpeed 113 (150 ms per cell less 25%)")
	}
}

func TestMagnumBreakKnocksBack(t *testing.T) {
	url, store := startServer(t)
	createChar(t, store, "Burner", func(c *game.CharData) {
		c.Job, c.BaseLevel, c.SP = game.JobSwordman, 20, 100
		c.Skills[game.SkillBash] = 5
		c.Skills[game.SkillMagnumBreak] = 1
	})
	c := dial(t, url)
	_, mobs := c.loginJob("Burner")
	east := mobs[helperMob][0] // the helpers stand at (11,11) and (9,11)
	west := mobs[helperMob][1]

	c.send(useSkill(game.SkillMagnumBreak, 0))
	hit := map[uint32]bool{}
	pushed := map[uint32][2]int{}
	c.readUntil("Magnum Break", func(op uint16, r *proto.Reader) bool {
		switch op {
		case proto.OpDamage:
			if d := readDamage(r); d.skill == game.SkillMagnumBreak {
				hit[d.target] = true
			}
		case proto.OpPlayerStop:
			id := r.U32()
			pushed[id] = [2]int{int(r.U16()), int(r.U16())}
		}
		return len(hit) == 2 && len(pushed) >= 1
	})
	if !hit[east] || !hit[west] {
		t.Fatalf("both helpers should be hit: %v", hit)
	}
	for id, at := range pushed {
		if at != [2]int{13, 13} && at != [2]int{7, 13} {
			t.Fatalf("monster %d knocked to %v", id, at)
		}
	}
}

func TestDiscountLowersShopPrices(t *testing.T) {
	url, store := startServer(t)
	createChar(t, store, "Trader", func(c *game.CharData) {
		c.Job = game.JobMerchant
		c.Skills[game.SkillDiscount] = 1
		c.Skills[game.SkillOvercharge] = 1
	})
	c := dial(t, url)
	c.loginJob("Trader")
	c.send(proto.NewPacket(proto.OpNpcClick).U32(shopNpc).Bytes())
	r := c.expect(proto.OpShopOpen)
	r.U32()
	_ = r.String()
	prices := map[int]int{}
	for range int(r.U16()) {
		id := int(r.U16())
		prices[id] = int(r.U32())
		r.U8() // forged element (vendors)
	}
	if prices[1201] != 46 { // Knife: 50 less 7%
		t.Fatalf("Knife price with Discount 1 = %d", prices[1201])
	}
	if rate := int(r.U16()); rate != 107 {
		t.Fatalf("sell rate with Overcharge 1 = %d", rate)
	}
}
