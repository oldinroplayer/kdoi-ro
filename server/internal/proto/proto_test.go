package proto

import "testing"

func TestDecodeClientPackets(t *testing.T) {
	pkt, err := Decode(NewPacket(OpLogin).String("Alice").String("pw1234").Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := pkt.(*Login); !ok || l.Name != "Alice" || l.Password != "pw1234" {
		t.Fatalf("login = %#v", pkt)
	}

	pkt, err = Decode(NewPacket(OpMove).U16(160).U16(95).Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := pkt.(*Move); !ok || m.X != 160 || m.Y != 95 {
		t.Fatalf("move = %#v", pkt)
	}
}

func TestDecodeErrors(t *testing.T) {
	for name, b := range map[string][]byte{
		"empty":         {},
		"unknown":       NewPacket(0x7777).Bytes(),
		"short move":    NewPacket(OpMove).U16(1).Bytes(),
		"short string":  append(NewPacket(OpLogin).Bytes(), 10, 'a'),
		"server opcode": PlayerDisappear(1, GoneOut),
	} {
		if _, err := Decode(b); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestPlayerMoveLayout(t *testing.T) {
	r := NewReader(PlayerMove(7, 1, 2, [][2]int{{2, 3}, {3, 3}}))
	got := []uint32{uint32(r.U16()), r.U32(), uint32(r.U16()), uint32(r.U16()), uint32(r.U16()),
		uint32(r.U16()), uint32(r.U16()), uint32(r.U16()), uint32(r.U16())}
	want := []uint32{uint32(OpPlayerMove), 7, 1, 2, 2, 2, 3, 3, 3}
	if r.Err() != nil {
		t.Fatal(r.Err())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field %d = %d, want %d (all: %v)", i, got[i], want[i], got)
		}
	}
}
