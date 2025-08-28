package protocol

import (
    "encoding/binary"
    "fmt"
    "log"
    "unicode/utf8"
    fecpkg "github.com/jancona/m17/pkg/fec"
)

type PacketType rune

const (
	PacketTypeRAW     PacketType = 0x00
	PacketTypeAX25    PacketType = 0x01
	PacketTypeAPRS    PacketType = 0x02
	PacketType6LoWPAN PacketType = 0x03
	PacketTypeIPv4    PacketType = 0x04
	PacketTypeSMS     PacketType = 0x05
	PacketTypeWinlink PacketType = 0x06
)

// M17 packet
type Packet struct {
	LSF     LSF
	Type    PacketType
	Payload []byte
	CRC     uint16
}

func NewPacketFromBytes(buf []byte) Packet {
	var p Packet
	p.LSF = NewLSFFromBytes(buf[:LSFLen])
	t, size := utf8.DecodeRune(buf[LSFLen:])
	p.Type = PacketType(t)
	p.Payload = buf[LSFLen+size : len(buf)-2]
	_, err := binary.Decode(buf[len(buf)-2:], binary.BigEndian, &p.CRC)
	if err != nil {
		// should never happen
		log.Printf("[ERROR] Error decoding CRC: %v", err)
	}
	return p
}

func NewPacket(dst, src string, t PacketType, data []byte) (*Packet, error) {
    lsf, err := NewLSF(dst, src, LSFTypePacket, LSFDataTypeData, 0)
    if err != nil {
        return nil, fmt.Errorf("failed to create LSF for Packet: %w", err)
    }
    lsf.CalcCRC()
    p := Packet{
        LSF:  lsf,
        Type: t,
    }
    p.Payload = append(p.Payload, data...)
    pb := p.PayloadBytes()
    p.CRC = fecpkg.CRC(pb[:len(pb)-2])
    return &p, nil
}

// Convert this Packet to a byte slice suitable for transmission
func (p *Packet) ToBytes() []byte {
	pb := p.PayloadBytes()
	b := make([]byte, LSFLen+len(pb))
	copy(b[:LSFLen], p.LSF.ToBytes())
	copy(b[LSFLen:], pb)
	return b
}

// Convert the payload (type, message and CRC) to a byte slice suitable for transmission
func (p *Packet) PayloadBytes() []byte {
	b := utf8.AppendRune(nil, rune(p.Type))
	b = append(b, p.Payload...)
	b, err := binary.Append(b, binary.BigEndian, p.CRC)
	if err != nil {
		// should never happen
		log.Printf("[ERROR] Error encoding CRC: %v", err)
	}
	return b
}

// Check if the CRC is correct
func (p *Packet) CheckCRC() bool {
    a := p.PayloadBytes()
    return fecpkg.CRC(a) == 0
}

func (p Packet) String() string {
	var pl string
	if p.Type == 5 {
		pl = string(p.Payload[:len(p.Payload)-1])
	} else {
		pl = fmt.Sprintf("%#v", p.Payload)
	}

	return fmt.Sprintf(`{
	LSF: %s,
	Type: %#v,
	Payload: %s,
	CRC: %#v
}`, p.LSF, p.Type, pl, p.CRC)
}
