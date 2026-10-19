package world

import (
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"kdoiro/server/internal/proto"
)

const (
	// chatRange is how far (in cells) a public chat line carries, like
	// eAthena's AREA_SIZE.
	chatRange = 14
	// maxChatRunes caps one chat line.
	maxChatRunes = 120
	// Chat room limits (RO allows 2-20 people and a short title).
	minRoomLimit   = 2
	maxRoomLimit   = 20
	maxTitleRunes  = 36
	maxPasswordLen = 8
	// roomJoinRange is how close you must be to a chat room's sign to join.
	roomJoinRange = chatRange
)

// chatRoom is an RO chat room: a sign over its owner's head that others can
// click to join. Members chat only with each other and can't walk away.
type chatRoom struct {
	id       uint32
	m        *mapInstance
	title    string
	limit    int
	public   bool
	password string
	members  []*Player // members[0] is the owner
	banned   map[string]bool
}

func (r *chatRoom) owner() *Player { return r.members[0] }

func (r *chatRoom) send(packet []byte) {
	for _, p := range r.members {
		p.out.Send(packet)
	}
}

func (r *chatRoom) signPacket() []byte {
	return proto.RoomSign(r.id, r.owner().id, r.title, len(r.members), r.limit, r.public)
}

func (r *chatRoom) names() []string {
	names := make([]string, len(r.members))
	for i, p := range r.members {
		names[i] = p.Name
	}
	return names
}

// cleanText trims a chat line, turns control characters (tabs, newlines) into
// spaces and caps its length.
func cleanText(s string, maxRunes int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxRunes {
		s = string([]rune(s)[:maxRunes])
	}
	return s
}

// Chat says a line: to the player's chat room if it's in one, otherwise out
// loud to everyone nearby (including the speaker, whose client shows it too).
func (w *World) Chat(id uint32, text string) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		text = cleanText(text, maxChatRunes)
		if text == "" {
			return
		}
		if r := p.room; r != nil {
			r.send(proto.ChatMessage(p.id, proto.ChatRoom, p.Name, text))
			return
		}
		packet := proto.ChatMessage(p.id, proto.ChatPublic, p.Name, text)
		if w.serverChat {
			for _, other := range w.players {
				other.out.Send(packet)
			}
			return
		}
		for _, other := range p.m.players {
			if chebyshev(p.x, p.y, other.x, other.y) <= chatRange {
				other.out.Send(packet)
			}
		}
	})
}

// ServerChat reports whether public chat reaches the whole server.
func (w *World) ServerChat() bool {
	var on bool
	w.do(func() { on = w.serverChat })
	return on
}

// SetServerChat switches public chat between everyone online (true) and only
// players nearby (false, RO's usual), and tells everyone.
func (w *World) SetServerChat(on bool) error {
	return w.do(func() {
		w.serverChat = on
		msg := "Chat is now local: only players nearby hear you."
		if on {
			msg = "Chat is now server-wide: everyone online hears you."
		}
		packet := proto.Message(msg)
		for _, p := range w.players {
			p.out.Send(packet)
		}
	})
}

// Whisper sends a private message to a character by name, wherever it is.
func (w *World) Whisper(id uint32, to, text string) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		text = cleanText(text, maxChatRunes)
		if text == "" {
			return
		}
		target := w.players[w.online[strings.ToLower(strings.TrimSpace(to))]]
		switch {
		case target == nil:
			p.out.Send(proto.WhisperResult(proto.WhisperOffline, to))
		case target == p:
			p.out.Send(proto.WhisperResult(proto.WhisperSelf, to))
		default:
			target.out.Send(proto.WhisperFrom(p.Name, text))
			p.out.Send(proto.WhisperResult(proto.WhisperSent, target.Name))
		}
	})
}

// validRoom cleans chat room settings, or reports them invalid.
func validRoom(s proto.RoomSettings) (title, password string, limit int, ok bool) {
	title = cleanText(s.Title, maxTitleRunes)
	password = cleanText(s.Password, maxPasswordLen)
	limit = int(s.Limit)
	if title == "" || limit < minRoomLimit || limit > maxRoomLimit || (!s.Public && password == "") {
		return "", "", 0, false
	}
	if s.Public {
		password = ""
	}
	return title, password, limit, true
}

// CreateRoom opens a chat room owned by the player.
func (w *World) CreateRoom(id uint32, s proto.RoomSettings) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		if p.room != nil {
			p.out.Send(proto.RoomResult(proto.RoomAlreadyIn))
			return
		}
		if p.dead || p.casting != nil {
			p.out.Send(proto.RoomResult(proto.RoomCantNow))
			return
		}
		title, password, limit, ok := validRoom(s)
		if !ok {
			p.out.Send(proto.RoomResult(proto.RoomBadSettings))
			return
		}
		w.halt(&p.unit)
		p.target, p.pending = 0, nil
		w.nextID++
		r := &chatRoom{
			id: w.nextID, m: p.m, title: title, limit: limit, public: s.Public, password: password,
			members: []*Player{p}, banned: map[string]bool{},
		}
		w.rooms[r.id] = r
		p.room = r
		p.out.Send(proto.RoomEnter(r.id, r.title, r.limit, r.public, p.Name, r.names()))
		w.broadcast(r.m, r.signPacket())
	})
}

// JoinRoom enters someone's chat room.
func (w *World) JoinRoom(id, roomID uint32, password string) error {
	return w.do(func() {
		p, r := w.players[id], w.rooms[roomID]
		if p == nil {
			return
		}
		switch {
		case p.room != nil:
			p.out.Send(proto.RoomResult(proto.RoomAlreadyIn))
		case r == nil || r.m != p.m || chebyshev(p.x, p.y, r.owner().x, r.owner().y) > roomJoinRange:
			p.out.Send(proto.RoomResult(proto.RoomNotFound))
		case p.dead:
			p.out.Send(proto.RoomResult(proto.RoomCantNow))
		case r.banned[strings.ToLower(p.Name)]:
			p.out.Send(proto.RoomResult(proto.RoomBanned))
		case len(r.members) >= r.limit:
			p.out.Send(proto.RoomResult(proto.RoomFull))
		case !r.public && password != r.password:
			p.out.Send(proto.RoomResult(proto.RoomWrongPassword))
		default:
			w.halt(&p.unit)
			p.target, p.pending = 0, nil
			w.cancelCast(p)
			r.send(proto.RoomMember(proto.MemberJoined, p.Name))
			r.members = append(r.members, p)
			p.room = r
			p.out.Send(proto.RoomEnter(r.id, r.title, r.limit, r.public, r.owner().Name, r.names()))
			w.broadcast(r.m, r.signPacket())
		}
	})
}

// LeaveRoom takes the player out of its chat room.
func (w *World) LeaveRoom(id uint32) error {
	return w.do(func() {
		if p := w.players[id]; p != nil && p.room != nil {
			w.leaveRoom(p, proto.LeftByChoice)
		}
	})
}

// leaveRoom removes p from its room. When the owner leaves, the next member
// takes over; the room closes when nobody is left.
func (w *World) leaveRoom(p *Player, reason uint8) {
	r := p.room
	if r == nil {
		return
	}
	p.room = nil
	wasOwner := r.owner() == p
	r.members = slices.DeleteFunc(r.members, func(m *Player) bool { return m == p })
	p.out.Send(proto.RoomLeft(reason))
	if len(r.members) == 0 {
		delete(w.rooms, r.id)
		w.broadcast(r.m, proto.RoomGone(r.id))
		return
	}
	change := proto.MemberLeft
	if reason == proto.LeftKicked {
		change = proto.MemberKicked
	}
	r.send(proto.RoomMember(change, p.Name))
	if wasOwner {
		r.send(proto.RoomSettingsChanged(r.title, r.limit, r.public, r.owner().Name))
	}
	w.broadcast(r.m, r.signPacket())
}

// ownedRoom returns the room p owns, or nil.
func ownedRoom(p *Player) *chatRoom {
	if p == nil || p.room == nil || p.room.owner() != p {
		return nil
	}
	return p.room
}

func (r *chatRoom) member(name string) *Player {
	for _, m := range r.members {
		if strings.EqualFold(m.Name, name) {
			return m
		}
	}
	return nil
}

// KickFromRoom lets a room's owner throw a member out; they can't come back.
func (w *World) KickFromRoom(id uint32, name string) error {
	return w.do(func() {
		r := ownedRoom(w.players[id])
		if r == nil {
			return
		}
		if m := r.member(name); m != nil && m != r.owner() {
			r.banned[strings.ToLower(m.Name)] = true
			w.leaveRoom(m, proto.LeftKicked)
		}
	})
}

// UpdateRoom changes a room's settings (owner only).
func (w *World) UpdateRoom(id uint32, s proto.RoomSettings) error {
	return w.do(func() {
		p := w.players[id]
		r := ownedRoom(p)
		if r == nil {
			return
		}
		title, password, limit, ok := validRoom(s)
		if !ok || limit < len(r.members) {
			p.out.Send(proto.RoomResult(proto.RoomBadSettings))
			return
		}
		r.title, r.password, r.limit, r.public = title, password, limit, s.Public
		r.send(proto.RoomSettingsChanged(r.title, r.limit, r.public, p.Name))
		w.broadcast(r.m, r.signPacket())
	})
}

// GiveRoom hands a room to another member (owner only).
func (w *World) GiveRoom(id uint32, name string) error {
	return w.do(func() {
		r := ownedRoom(w.players[id])
		if r == nil {
			return
		}
		m := r.member(name)
		if m == nil || m == r.owner() {
			return
		}
		i := slices.Index(r.members, m)
		r.members[0], r.members[i] = r.members[i], r.members[0]
		r.send(proto.RoomSettingsChanged(r.title, r.limit, r.public, m.Name))
		w.broadcast(r.m, r.signPacket())
	})
}

// sendRoomSigns shows a player arriving on a map the chat rooms there.
func (w *World) sendRoomSigns(p *Player) {
	for _, r := range w.rooms {
		if r.m == p.m {
			p.out.Send(r.signPacket())
		}
	}
}

// emoteCooldown keeps emoticons from being spammed (RO has a similar delay).
const emoteCooldown = time.Second

// Emote shows an emoticon over the player's head to everyone nearby.
func (w *World) Emote(id uint32, kind int) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil || p.dead || kind < 0 || kind >= proto.Emotions {
			return
		}
		now := time.Now()
		if now.Before(p.nextEmote) {
			return
		}
		p.nextEmote = now.Add(emoteCooldown)
		packet := proto.Emotion(p.id, uint8(kind))
		for _, other := range p.m.players {
			if chebyshev(p.x, p.y, other.x, other.y) <= chatRange {
				other.out.Send(packet)
			}
		}
	})
}
