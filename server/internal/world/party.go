package world

import (
	"log/slog"
	"slices"
	"strings"
	"time"

	"kdoiro/server/internal/proto"
)

const (
	// MaxPartySize is RO's 12 members.
	MaxPartySize = 12
	// evenShareLevelGap: even EXP share needs every member within 10 base levels.
	evenShareLevelGap = 10
	maxPartyNameRunes = 23
	// partyStatusInterval is how often members' maps, positions and HP refresh.
	partyStatusInterval = time.Second
)

// PartyRecord is a party as stored between server runs.
type PartyRecord struct {
	ID      uint32
	Name    string
	Leader  string   // a member's character name
	Members []string // character names, in joining order
	EvenEXP bool     // even share instead of each take
}

// PartyStore saves parties; the world calls it whenever one changes.
type PartyStore interface {
	SaveParty(PartyRecord) error
	DeleteParty(id uint32) error
}

// party is a party in play. Members are kept by character name so offline
// members stay in it, as in RO.
type party struct {
	PartyRecord
	lastStatus string // what members were last sent, to skip unchanged updates
}

func lower(name string) string { return strings.ToLower(name) }

func (pt *party) has(name string) bool {
	return slices.ContainsFunc(pt.Members, func(m string) bool { return strings.EqualFold(m, name) })
}

func (pt *party) isLeader(name string) bool { return strings.EqualFold(pt.Leader, name) }

// partyOf returns the party a character belongs to, or nil.
func (w *World) partyOf(name string) *party {
	return w.parties[w.partyByMember[lower(name)]]
}

// online returns a party member's player if logged in.
func (w *World) onlinePlayer(name string) *Player {
	return w.players[w.online[lower(name)]]
}

func (w *World) saveParty(pt *party) {
	if w.partyStore == nil {
		return
	}
	if err := w.partyStore.SaveParty(pt.PartyRecord); err != nil {
		slog.Error("saving party", "party", pt.Name, "err", err)
	}
}

// partyMessage tells every online member something.
func (w *World) partyMessage(pt *party, text string) {
	for _, name := range pt.Members {
		if p := w.onlinePlayer(name); p != nil {
			p.out.Send(proto.Message(text))
		}
	}
}

// CreateParty founds a party with the player as leader (RO's /organize).
func (w *World) CreateParty(id uint32, name string) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		name = cleanText(name, maxPartyNameRunes)
		switch {
		case w.partyOf(p.Name) != nil:
			p.out.Send(proto.Message("You're already in a party. Leave it first."))
			return
		case name == "":
			p.out.Send(proto.Message("Give your party a name."))
			return
		}
		for _, other := range w.parties {
			if strings.EqualFold(other.Name, name) {
				p.out.Send(proto.Message("That party name is taken."))
				return
			}
		}
		w.nextPartyID++
		pt := &party{PartyRecord: PartyRecord{ID: w.nextPartyID, Name: name, Leader: p.Name, Members: []string{p.Name}}}
		w.parties[pt.ID] = pt
		w.partyByMember[lower(p.Name)] = pt.ID
		w.saveParty(pt)
		p.out.Send(proto.Message("You founded the party " + name + "."))
		w.sendPartyInfo(pt)
	})
}

// InviteToParty asks an online character to join the leader's party.
func (w *World) InviteToParty(id uint32, targetName string) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		pt := w.partyOf(p.Name)
		target := w.onlinePlayer(strings.TrimSpace(targetName))
		switch {
		case pt == nil:
			p.out.Send(proto.Message("You're not in a party. Create one first."))
		case !pt.isLeader(p.Name):
			p.out.Send(proto.Message("Only the party leader can invite."))
		case target == nil:
			p.out.Send(proto.Message(targetName + " isn't online."))
		case target == p || w.partyOf(target.Name) != nil:
			p.out.Send(proto.Message(target.Name + " is already in a party."))
		case len(pt.Members) >= MaxPartySize:
			p.out.Send(proto.Message("Your party is full."))
		case target.partyInvite != 0:
			p.out.Send(proto.Message(target.Name + " is considering another invitation."))
		default:
			target.partyInvite = pt.ID
			target.out.Send(proto.PartyInvited(pt.ID, pt.Name, p.Name))
			p.out.Send(proto.Message("You invited " + target.Name + " to the party."))
		}
	})
}

// ReplyPartyInvite accepts or declines an invitation.
func (w *World) ReplyPartyInvite(id, partyID uint32, accept bool) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil || p.partyInvite == 0 || p.partyInvite != partyID {
			return
		}
		p.partyInvite = 0
		pt := w.parties[partyID]
		if pt == nil {
			p.out.Send(proto.Message("That party no longer exists."))
			return
		}
		if !accept {
			if leader := w.onlinePlayer(pt.Leader); leader != nil {
				leader.out.Send(proto.Message(p.Name + " declined to join the party."))
			}
			return
		}
		if w.partyOf(p.Name) != nil || len(pt.Members) >= MaxPartySize {
			p.out.Send(proto.Message("You can't join that party."))
			return
		}
		pt.Members = append(pt.Members, p.Name)
		w.partyByMember[lower(p.Name)] = pt.ID
		w.saveParty(pt)
		w.partyMessage(pt, p.Name+" joined the party.")
		w.sendPartyInfo(pt)
	})
}

// LeaveParty takes the player out of its party.
func (w *World) LeaveParty(id uint32) error {
	return w.do(func() {
		if p := w.players[id]; p != nil {
			if pt := w.partyOf(p.Name); pt != nil {
				w.removeFromParty(pt, p.Name, p.Name+" left the party.")
			}
		}
	})
}

// KickFromParty lets the leader remove a member.
func (w *World) KickFromParty(id uint32, name string) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		pt := w.partyOf(p.Name)
		if pt == nil || !pt.isLeader(p.Name) || strings.EqualFold(name, p.Name) || !pt.has(name) {
			return
		}
		w.removeFromParty(pt, name, name+" was removed from the party.")
	})
}

// removeFromParty drops a member; a leaving leader hands over to the next
// member (online ones first), and an empty party is disbanded.
func (w *World) removeFromParty(pt *party, name, notice string) {
	w.partyMessage(pt, notice)
	pt.Members = slices.DeleteFunc(pt.Members, func(m string) bool { return strings.EqualFold(m, name) })
	delete(w.partyByMember, lower(name))
	if p := w.onlinePlayer(name); p != nil {
		p.out.Send(proto.PartyLeft())
	}
	if len(pt.Members) == 0 {
		delete(w.parties, pt.ID)
		if w.partyStore != nil {
			if err := w.partyStore.DeleteParty(pt.ID); err != nil {
				slog.Error("deleting party", "party", pt.Name, "err", err)
			}
		}
		return
	}
	if strings.EqualFold(pt.Leader, name) {
		pt.Leader = pt.Members[0]
		for _, m := range pt.Members {
			if w.onlinePlayer(m) != nil {
				pt.Leader = m
				break
			}
		}
		w.partyMessage(pt, pt.Leader+" now leads the party.")
	}
	w.saveParty(pt)
	w.sendPartyInfo(pt)
}

// SetPartyEXP switches between each take and even share (leader only).
func (w *World) SetPartyEXP(id uint32, even bool) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		pt := w.partyOf(p.Name)
		if pt == nil || !pt.isLeader(p.Name) {
			return
		}
		if even && !w.levelsClose(pt.Members) {
			p.out.Send(proto.Message("Even share needs every online member within 10 base levels."))
			return
		}
		pt.EvenEXP = even
		w.saveParty(pt)
		if even {
			w.partyMessage(pt, "EXP is now shared evenly.")
		} else {
			w.partyMessage(pt, "EXP is now each take.")
		}
		w.sendPartyInfo(pt)
	})
}

// levelsClose reports whether the online members' base levels are within
// evenShareLevelGap of each other.
func (w *World) levelsClose(members []string) bool {
	lo, hi := 1<<30, 0
	for _, name := range members {
		if p := w.onlinePlayer(name); p != nil {
			lo, hi = min(lo, p.data.BaseLevel), max(hi, p.data.BaseLevel)
		}
	}
	return hi-lo <= evenShareLevelGap
}

// PartyChat says something to every online party member.
func (w *World) PartyChat(id uint32, text string) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		pt := w.partyOf(p.Name)
		text = cleanText(text, maxChatRunes)
		if pt == nil {
			p.out.Send(proto.Message("You're not in a party."))
			return
		}
		if text == "" {
			return
		}
		packet := proto.ChatMessage(p.id, proto.ChatParty, p.Name, text)
		for _, name := range pt.Members {
			if m := w.onlinePlayer(name); m != nil {
				m.out.Send(packet)
			}
		}
	})
}

// partyInfo builds the party window's contents.
func (w *World) partyInfo(pt *party) []byte {
	members := make([]proto.PartyMember, len(pt.Members))
	for i, name := range pt.Members {
		m := proto.PartyMember{Name: name}
		if p := w.onlinePlayer(name); p != nil {
			d := p.data.Derive()
			m = proto.PartyMember{Name: p.Name, Online: true, Map: p.m.name, X: p.x, Y: p.y,
				HP: p.data.HP, MaxHP: d.MaxHP, Job: int(p.data.Job), BaseLevel: p.data.BaseLevel}
		}
		members[i] = m
	}
	return proto.PartyInfo(pt.ID, pt.Name, pt.Leader, pt.EvenEXP, members)
}

// sendPartyInfo refreshes every online member's party window.
func (w *World) sendPartyInfo(pt *party) {
	packet := w.partyInfo(pt)
	pt.lastStatus = string(packet)
	for _, name := range pt.Members {
		if p := w.onlinePlayer(name); p != nil {
			p.out.Send(packet)
		}
	}
}

// tickParties refreshes members' positions and HP once a second, for parties
// where something changed.
func (w *World) tickParties(now time.Time) {
	if now.Before(w.nextPartyStatus) {
		return
	}
	w.nextPartyStatus = now.Add(partyStatusInterval)
	for _, pt := range w.parties {
		online := 0
		for _, name := range pt.Members {
			if w.onlinePlayer(name) != nil {
				online++
			}
		}
		if online == 0 {
			continue
		}
		if packet := w.partyInfo(pt); string(packet) != pt.lastStatus {
			w.sendPartyInfo(pt)
		}
	}
}

// partyIDOf is a player's party ID (0 = none), e.g. to share loot rights.
func (w *World) partyIDOf(p *Player) uint32 {
	if p == nil {
		return 0
	}
	return w.partyByMember[lower(p.Name)]
}

// shareEXP gives out a kill's EXP. Members of an even-share party pool what
// they earned and split it evenly among the members on the monster's map who
// are alive and whose levels are close; everyone else keeps their own share.
func (w *World) shareEXP(m *mapInstance, earned map[*Player][2]int) {
	pools := map[*party][2]int{}
	for p, exp := range earned {
		if pt := w.partyOf(p.Name); pt != nil && pt.EvenEXP {
			pool := pools[pt]
			pools[pt] = [2]int{pool[0] + exp[0], pool[1] + exp[1]}
			continue
		}
		w.giveExp(p, exp[0], exp[1])
	}
	for pt, pool := range pools {
		var takers []*Player
		for _, name := range pt.Members {
			if p := w.onlinePlayer(name); p != nil && p.m == m && !p.dead {
				takers = append(takers, p)
			}
		}
		names := make([]string, len(takers))
		for i, p := range takers {
			names[i] = p.Name
		}
		if len(takers) == 0 || !w.levelsClose(names) {
			// Too far apart now: back to each take for this kill.
			for p, exp := range earned {
				if w.partyOf(p.Name) == pt {
					w.giveExp(p, exp[0], exp[1])
				}
			}
			continue
		}
		for _, p := range takers {
			w.giveExp(p, pool[0]/len(takers), pool[1]/len(takers))
		}
	}
}

// partyLogin and partyLogout refresh the party when a member comes or goes.
func (w *World) partyLogin(p *Player) {
	if pt := w.partyOf(p.Name); pt != nil {
		w.sendPartyInfo(pt)
	}
}

func (w *World) partyLogout(p *Player) {
	p.partyInvite = 0
	if pt := w.partyOf(p.Name); pt != nil {
		w.sendPartyInfo(pt) // they've been removed from w.online already
	}
}
