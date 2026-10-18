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

// startServer runs a world + server on a 20x20 open "morocc" and returns its ws URL.
func startServer(t *testing.T) (string, *account.Store) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store, err := account.Open(filepath.Join(t.TempDir(), "chars.json"))
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(map[string]*gat.Map{"morocc": openGrid(t, 20, 20)})
	go w.Run(ctx)
	srv := New(ctx, store, w, Spawn{Map: "morocc", X: 10, Y: 10}, nil)
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

// expect reads the next packet, checks its opcode and returns a reader past it.
func (c *client) expect(op uint16) *proto.Reader {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		c.t.Fatalf("waiting for %#04x: %v", op, err)
	}
	r := proto.NewReader(data)
	if got := r.U16(); got != op {
		c.t.Fatalf("got opcode %#04x, want %#04x", got, op)
	}
	return r
}

func (c *client) expectLogin(code uint8) {
	c.t.Helper()
	if got := c.expect(proto.OpLoginResult).U8(); got != code {
		c.t.Fatalf("login result = %d, want %d", got, code)
	}
}

// expectEnter returns the self ID and position from EnterMap.
func (c *client) expectEnter() (id uint32, x, y int) {
	c.t.Helper()
	r := c.expect(proto.OpEnterMap)
	id = r.U32()
	if m := r.String(); m != "morocc" {
		c.t.Fatalf("map = %q", m)
	}
	return id, int(r.U16()), int(r.U16())
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
