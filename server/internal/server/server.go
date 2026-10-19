// Package server accepts WebSocket connections and runs one session per client.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"kdoiro/server/internal/account"
	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
	"kdoiro/server/internal/world"
)

const (
	readLimit        = 1024
	loginTimeout     = 30 * time.Second
	maxLoginAttempts = 5
	writeTimeout     = 10 * time.Second
	outboxSize       = 256
	minMoveInterval  = 100 * time.Millisecond
	// Chat flood guard: at most chatBurst lines per chatWindow; extra lines
	// are dropped with a notice.
	chatBurst  = 5
	chatWindow = 3 * time.Second
)

// Spawn is where new characters (and characters on unknown maps) appear.
type Spawn struct {
	Map  string
	X, Y int
}

type Server struct {
	ctx            context.Context // cancelling it ends every session
	store          *account.Store
	world          *world.World
	spawn          Spawn
	originPatterns []string
	wg             sync.WaitGroup

	mu sync.Mutex
	// players maps each logged-in session to its character's name.
	players map[*session]string
}

// New creates a server. originPatterns lists allowed browser origins
// (host patterns such as "localhost:*"); same-origin is always allowed.
func New(ctx context.Context, store *account.Store, w *world.World, spawn Spawn, originPatterns []string) *Server {
	return &Server{ctx: ctx, store: store, world: w, spawn: spawn, originPatterns: originPatterns, players: map[*session]string{}}
}

// ResumeTTL is how long a resume token from an update stays good: the wait
// the client is told, plus time for a slow page reload.
const ResumeTTL = 2 * time.Minute

// BeginUpdate tells every logged-in player the server is restarting for an
// update and hands each a one-time resume token, then closes their
// connections with CloseUpdating. Closing makes each session save its
// character as usual. Call it once new connections are no longer accepted, so
// the clients' wait ends on the new server, not this one. It returns once
// every connection has closed (or timed out closing).
func (s *Server) BeginUpdate(waitSeconds int) {
	s.mu.Lock()
	sessions := make(map[*session]string, len(s.players))
	for sess, name := range s.players {
		sessions[sess] = name
	}
	s.mu.Unlock()

	var wg sync.WaitGroup
	for sess, name := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := s.store.IssueResumeToken(name, ResumeTTL)
			if err != nil {
				sess.log.Error("resume token", "err", err)
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
				err = sess.conn.Write(ctx, websocket.MessageBinary, proto.ServerUpdating(waitSeconds, token))
				cancel()
				if err != nil {
					sess.log.Warn("send update notice", "err", err)
				}
			}
			sess.conn.Close(proto.CloseUpdating, "server updating")
		}()
	}
	wg.Wait()
}

// Wait blocks until every session has ended and saved its character.
func (s *Server) Wait() { s.wg.Wait() }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Add before Accept hijacks the connection, so http.Server.Shutdown
	// can't return before this session is counted.
	s.wg.Add(1)
	defer s.wg.Done()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.originPatterns})
	if err != nil {
		return
	}
	conn.SetReadLimit(readLimit)
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	sess := &session{conn: conn, out: make(chan []byte, outboxSize), cancel: cancel, log: slog.With("remote", r.RemoteAddr), done: make(chan struct{})}
	defer close(sess.done)
	status, reason := s.run(ctx, sess)
	conn.Close(status, reason)
}

// replaceWait is how long a login waits for the session it takes over to
// save its character and end.
const replaceWait = 10 * time.Second

// takeOver ends the session playing a character (a newer login wins) and
// waits for it to save. It reports false if the old session didn't end in time.
func (s *Server) takeOver(name string) bool {
	s.mu.Lock()
	var old *session
	for sess, n := range s.players {
		if strings.EqualFold(n, name) {
			old = sess
			break
		}
	}
	s.mu.Unlock()
	if old == nil {
		return true // already gone
	}
	old.log.Info("replaced by a new login")
	go old.conn.Close(proto.CloseReplaced, "Your character logged in from another place.")
	select {
	case <-old.done:
		return true
	case <-time.After(replaceWait):
		return false
	}
}

type session struct {
	conn   *websocket.Conn
	out    chan []byte
	cancel context.CancelFunc
	log    *slog.Logger
	// done closes once the session has ended and saved its character.
	done chan struct{}
}

// Send queues a packet; a client too slow to keep up is disconnected.
func (sess *session) Send(packet []byte) {
	select {
	case sess.out <- packet:
	default:
		sess.log.Warn("outbox full, disconnecting")
		sess.cancel()
	}
}

func (sess *session) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case packet := <-sess.out:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := sess.conn.Write(wctx, websocket.MessageBinary, packet)
			cancel()
			if err != nil {
				sess.cancel()
				return
			}
		}
	}
}

func (s *Server) run(ctx context.Context, sess *session) (websocket.StatusCode, string) {
	char, id, status, reason := s.login(ctx, sess)
	if id == 0 {
		return status, reason
	}
	sess.log = sess.log.With("char", char.Name, "id", id)
	sess.log.Info("joined")
	s.mu.Lock()
	s.players[sess] = char.Name
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.players, sess)
		s.mu.Unlock()
		data, ok := s.world.Leave(id)
		if ok {
			if err := s.store.Save(char.Name, data); err != nil {
				sess.log.Error("save character", "err", err)
			}
		}
		sess.log.Info("left", "map", data.Map, "x", data.X, "y", data.Y, "zeny", data.Zeny)
	}()

	go sess.writeLoop(ctx)

	var lastMove time.Time
	var chatTimes []time.Time // when recent chat lines were sent
	chatAllowed := func() bool {
		now := time.Now()
		for len(chatTimes) > 0 && now.Sub(chatTimes[0]) > chatWindow {
			chatTimes = chatTimes[1:]
		}
		if len(chatTimes) >= chatBurst {
			sess.Send(proto.Message("You're talking too fast. Wait a moment."))
			return false
		}
		chatTimes = append(chatTimes, now)
		return true
	}
	for {
		pkt, err := readPacket(ctx, sess.conn)
		if err != nil {
			return statusFor(err)
		}
		switch p := pkt.(type) {
		case *proto.Move:
			if now := time.Now(); now.Sub(lastMove) >= minMoveInterval {
				lastMove = now
				err = s.world.Move(id, int(p.X), int(p.Y))
			}
		case *proto.NpcClick:
			err = s.world.ClickNpc(id, p.NpcID)
		case *proto.ShopBuy:
			err = s.world.Buy(id, p.NpcID, p.Lines)
		case *proto.Equip:
			err = s.world.Equip(id, int(p.Index))
		case *proto.Unequip:
			err = s.world.Unequip(id, int(p.Index))
		case *proto.StatusUp:
			err = s.world.RaiseStat(id, int(p.Stat))
		case *proto.Attack:
			err = s.world.Attack(id, p.TargetID)
		case *proto.UseItem:
			err = s.world.UseItem(id, int(p.Index))
		case *proto.Respawn:
			err = s.world.Respawn(id)
		case *proto.Sit:
			err = s.world.Sit(id, p.Sit)
		case *proto.Turn:
			err = s.world.Turn(id, p.Dir)
		case *proto.NpcReply:
			err = s.world.NpcReply(id, p.NpcID, p.Choice)
		case *proto.Learn:
			err = s.world.LearnSkill(id, int(p.SkillID))
		case *proto.UseSkill:
			err = s.world.UseSkill(id, int(p.SkillID), int(p.Level), p.TargetID)
		case *proto.ShopSell:
			err = s.world.Sell(id, p.NpcID, p.Lines)
		case *proto.Chat:
			if chatAllowed() {
				err = s.world.Chat(id, p.Text)
			}
		case *proto.Whisper:
			if chatAllowed() {
				err = s.world.Whisper(id, p.To, p.Text)
			}
		case *proto.RoomCreate:
			err = s.world.CreateRoom(id, p.RoomSettings)
		case *proto.RoomJoin:
			err = s.world.JoinRoom(id, p.RoomID, p.Password)
		case *proto.RoomLeave:
			err = s.world.LeaveRoom(id)
		case *proto.RoomKick:
			err = s.world.KickFromRoom(id, p.Name)
		case *proto.RoomUpdate:
			err = s.world.UpdateRoom(id, p.RoomSettings)
		case *proto.RoomOwner:
			err = s.world.GiveRoom(id, p.Name)
		case *proto.PickUp:
			err = s.world.PickUp(id, p.ItemID)
		case *proto.PartyCreate:
			err = s.world.CreateParty(id, p.Name)
		case *proto.PartyInvite:
			err = s.world.InviteToParty(id, p.Name)
		case *proto.PartyReply:
			err = s.world.ReplyPartyInvite(id, p.PartyID, p.Accept)
		case *proto.PartyLeave:
			err = s.world.LeaveParty(id)
		case *proto.PartyKick:
			err = s.world.KickFromParty(id, p.Name)
		case *proto.PartyEXP:
			err = s.world.SetPartyEXP(id, p.Even)
		case *proto.PartyChat:
			if chatAllowed() {
				err = s.world.PartyChat(id, p.Text)
			}
		case *proto.Emote:
			err = s.world.Emote(id, int(p.Type))
		case *proto.SetAppearance:
			err = s.world.SetAppearance(id, int(p.Style), int(p.Color), p.Female)
		case *proto.DropItem:
			err = s.world.DropItem(id, int(p.Index), int(p.Amount))
		case *proto.Compound:
			err = s.world.Compound(id, int(p.Card), int(p.Equip))
		default:
			return websocket.StatusPolicyViolation, "unexpected packet"
		}
		if err != nil {
			return websocket.StatusGoingAway, "server shutting down"
		}
	}
}

// login reads Login packets until one succeeds and the character has joined
// the world. It returns id 0 (plus a close status) on failure.
func (s *Server) login(ctx context.Context, sess *session) (account.Character, uint32, websocket.StatusCode, string) {
	lctx, cancel := context.WithTimeout(ctx, loginTimeout)
	defer cancel()

	start := game.NewChar(s.spawn.Map, s.spawn.X, s.spawn.Y)
	start.WearStartEquipment()
	for range maxLoginAttempts {
		pkt, err := readPacket(lctx, sess.conn)
		if err != nil {
			status, reason := statusFor(err)
			return account.Character{}, 0, status, reason
		}
		var char account.Character
		var created bool
		switch login := pkt.(type) {
		case *proto.Login:
			char, created, err = s.store.Authenticate(login.Name, login.Password, start)
		case *proto.Resume:
			// Back after an update: the token stands in for the password, once.
			char, err = s.store.Resume(login.Token)
		default:
			return account.Character{}, 0, websocket.StatusPolicyViolation, "login first"
		}
		if err != nil {
			code := loginCode(err)
			if code == proto.LoginServerError {
				sess.log.Error("authenticate", "err", err)
			}
			if err := sess.conn.Write(lctx, websocket.MessageBinary, proto.LoginResult(code)); err != nil {
				return account.Character{}, 0, websocket.StatusInternalError, "write failed"
			}
			continue
		}

		data := char.CharData
		if !s.world.HasMap(data.Map) {
			data.Map, data.X, data.Y = s.spawn.Map, s.spawn.X, s.spawn.Y
		}
		// Join queues EnterMap into the outbox; LoginResult is written
		// directly below, before writeLoop starts, so it arrives first.
		id, err := s.world.Join(char.Name, data, sess)
		if errors.Is(err, world.ErrAlreadyOnline) && s.takeOver(char.Name) {
			// The old session saved on its way out: carry on from that save.
			if fresh, loadErr := s.store.Load(char.Name); loadErr == nil {
				char, data = fresh, fresh.CharData
				if !s.world.HasMap(data.Map) {
					data.Map, data.X, data.Y = s.spawn.Map, s.spawn.X, s.spawn.Y
				}
				id, err = s.world.Join(char.Name, data, sess)
			} else {
				err = loadErr
			}
		}
		if err != nil {
			code := loginCode(err)
			if code == proto.LoginServerError {
				sess.log.Error("join", "err", err)
			}
			if err := sess.conn.Write(lctx, websocket.MessageBinary, proto.LoginResult(code)); err != nil {
				return account.Character{}, 0, websocket.StatusInternalError, "write failed"
			}
			continue
		}

		code := proto.LoginOK
		if created {
			code = proto.LoginCreated
			sess.log.Info("character created", "char", char.Name)
		}
		if err := sess.conn.Write(lctx, websocket.MessageBinary, proto.LoginResult(code)); err != nil {
			s.world.Leave(id)
			return account.Character{}, 0, websocket.StatusInternalError, "write failed"
		}
		return char, id, 0, ""
	}
	return account.Character{}, 0, websocket.StatusPolicyViolation, "too many login attempts"
}

func readPacket(ctx context.Context, conn *websocket.Conn) (any, error) {
	typ, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	if typ != websocket.MessageBinary {
		return nil, errBadPacket
	}
	pkt, err := proto.Decode(data)
	if err != nil {
		return nil, errBadPacket
	}
	return pkt, nil
}

var errBadPacket = errors.New("malformed packet")

func statusFor(err error) (websocket.StatusCode, string) {
	switch {
	case errors.Is(err, errBadPacket):
		return websocket.StatusPolicyViolation, "malformed packet"
	case errors.Is(err, context.DeadlineExceeded):
		return websocket.StatusPolicyViolation, "login timeout"
	case errors.Is(err, context.Canceled):
		return websocket.StatusGoingAway, "server shutting down"
	default:
		return websocket.StatusNormalClosure, ""
	}
}

func loginCode(err error) uint8 {
	switch {
	case errors.Is(err, account.ErrWrongPassword):
		return proto.LoginWrongPassword
	case errors.Is(err, account.ErrInvalidName):
		return proto.LoginInvalidName
	case errors.Is(err, account.ErrInvalidPassword):
		return proto.LoginInvalidPassword
	case errors.Is(err, world.ErrAlreadyOnline):
		return proto.LoginAlreadyOnline
	case errors.Is(err, account.ErrResumeFailed):
		return proto.LoginResumeFailed
	default:
		return proto.LoginServerError
	}
}
