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
	OpLogin uint16 = 0x0001
	OpMove  uint16 = 0x0002
)

// Server -> client opcodes.
const (
	OpLoginResult     uint16 = 0x8001
	OpEnterMap        uint16 = 0x8002
	OpPlayerAppear    uint16 = 0x8003
	OpPlayerDisappear uint16 = 0x8004
	OpPlayerMove      uint16 = 0x8005
	OpPlayerStop      uint16 = 0x8006
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

// Decode reads one client packet and returns *Login or *Move.
func Decode(b []byte) (any, error) {
	r := NewReader(b)
	op := r.U16()
	var pkt any
	switch op {
	case OpLogin:
		pkt = &Login{Name: r.String(), Password: r.String()}
	case OpMove:
		pkt = &Move{X: r.U16(), Y: r.U16()}
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
func EnterMap(selfID uint32, mapName string, x, y int, dir uint8, job uint16, speed uint16) []byte {
	return NewPacket(OpEnterMap).U32(selfID).String(mapName).
		U16(uint16(x)).U16(uint16(y)).U8(dir).U16(job).U16(speed).Bytes()
}

func PlayerAppear(id uint32, name string, job uint16, x, y int, dir uint8, speed uint16) []byte {
	return NewPacket(OpPlayerAppear).U32(id).String(name).U16(job).
		U16(uint16(x)).U16(uint16(y)).U8(dir).U16(speed).Bytes()
}

func PlayerDisappear(id uint32) []byte {
	return NewPacket(OpPlayerDisappear).U32(id).Bytes()
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

// PlayerStop snaps a player to a cell (end of walk or correction).
func PlayerStop(id uint32, x, y int) []byte {
	return NewPacket(OpPlayerStop).U32(id).U16(uint16(x)).U16(uint16(y)).Bytes()
}
