package m17

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
)

type LSFType byte

func (t LSFType) String() string {
	switch t {
	case 0:
		return "0 - Packet mode"
	case 1:
		return "1 - Stream mode"
	default:
		return "ERROR"
	}
}

type LSFDataType byte

func (t LSFDataType) String() string {
	switch t {
	case 0:
		return "0 - Reserved"
	case 1:
		return "1 - Data"
	case 2:
		return "2 - Voice"
	case 3:
		return "3 - Voice+Data"
	default:
		return "ERROR"
	}
}

type LSFEncryptionType byte

func (t LSFEncryptionType) String() string {
	switch t {
	case 0:
		return "0 - None"
	case 1:
		return "1 - Scrambler"
	case 2:
		return "2 - AES"
	case 3:
		return "3 - Other/Reserved"
	default:
		return "ERROR"
	}
}

type EncodedCallsign [EncodedCallsignLen]byte

func (e EncodedCallsign) Callsign() string {
	cs, _ := DecodeCallsign(e[:])
	return cs
}

const (
	LSFTypePacket LSFType = iota
	LSFTypeStream
)

const (
	LSFDataTypeReserved LSFDataType = iota
	LSFDataTypeData
	LSFDataTypeVoice
	LSFDataTypeVoiceData
)

const (
	LSFEncryptionTypeNone LSFEncryptionType = iota
	LSFEncryptionTypeScrambler
	LSFEncryptionTypeAES
	LSFEncryptionTypeOther
)

const (
	LSFLen = 30
	LSDLen = 28

	typeLen = 2
	metaLen = 112 / 8

	dstPos  = 0
	srcPos  = dstPos + EncodedCallsignLen
	typPos  = srcPos + EncodedCallsignLen
	metaPos = typPos + typeLen
	crcPos  = metaPos + metaLen
)

// Link Setup Frame
type LSF struct {
	Dst  EncodedCallsign
	Src  EncodedCallsign
	Type [typeLen]byte
	Meta [metaLen]byte
	CRC  [CRCLen]byte
}

func NewEmptyLSF() LSF {
	return LSF{}
}

func NewLSF(destCall, sourceCall string, t LSFType, dt LSFDataType, can byte) (LSF, error) {
	var err error
	lsf := NewEmptyLSF()
	dst, err := EncodeCallsign(destCall)
	if err != nil {
		return lsf, fmt.Errorf("bad dst callsign: %w", err)
	}
	lsf.Dst = *dst
	src, err := EncodeCallsign(sourceCall)
	if err != nil {
		return lsf, fmt.Errorf("bad src callsign: %w", err)
	}
	lsf.Src = *src
	if t == 0 {
		// Data Type is only defined for stream mode
		dt = 0
	}
	lsf.Type[0] = (can & 0x7)
	lsf.Type[1] = (byte(t) & 0x1) | ((byte(dt) & 0x3) << 1)
	return lsf, nil
}

func NewLSFFromBytes(buf []byte) LSF {
	var lsf LSF
	copy(lsf.Dst[:], buf[dstPos:srcPos])
	copy(lsf.Src[:], buf[srcPos:typPos])
	copy(lsf.Type[:], buf[typPos:metaPos])
	copy(lsf.Meta[:], buf[metaPos:crcPos])
	copy(lsf.CRC[:], buf[crcPos:crcPos+CRCLen])
	return lsf
}

func NewLSFFromLSD(lsd []byte) LSF {
	var lsf LSF
	copy(lsf.Dst[:], lsd[dstPos:srcPos])
	copy(lsf.Src[:], lsd[srcPos:typPos])
	copy(lsf.Type[:], lsd[typPos:metaPos])
	copy(lsf.Meta[:], lsd[metaPos:crcPos])
	lsf.CalcCRC()
	return lsf
}

// Convert this LSF to a byte slice suitable for transmission
func (l *LSF) ToLSDBytes() []byte {
	b := make([]byte, 0, LSDLen)

	b = append(b, l.Dst[:]...)
	b = append(b, l.Src[:]...)
	b = append(b, l.Type[:]...)
	b = append(b, l.Meta[:]...)
	return b
}

// Convert this LSF to a byte slice suitable for transmission
func (l *LSF) ToBytes() []byte {
	b := make([]byte, 0, LSFLen)

	b = append(b, l.Dst[:]...)
	b = append(b, l.Src[:]...)
	b = append(b, l.Type[:]...)
	b = append(b, l.Meta[:]...)
	b = append(b, l.CRC[:]...)
	// log.Printf("[DEBUG] LSF.ToBytes(): %#v", b)

	return b
}

// Calculate CRC for this LSF
func (l *LSF) CalcCRC() uint16 {
	a := l.ToBytes()
	crc := CRC(a[:LSFLen-CRCLen])
	crcb, _ := binary.Append(nil, binary.BigEndian, crc)
	copy(l.CRC[:], crcb)
	return crc
}

// Check if the CRC is correct
func (l *LSF) CheckCRC() bool {
	a := l.ToBytes()
	return CRC(a) == 0
}

func (l *LSF) LSFType() LSFType {
	return LSFType(l.Type[1] & 0x1)
}

func (l *LSF) DataType() LSFDataType {
	return LSFDataType((l.Type[1] >> 1) & 0x3)
}

func (l *LSF) EncryptionType() LSFEncryptionType {
	return LSFEncryptionType((l.Type[1] >> 3) & 0x3)
}

func (l *LSF) EncryptionSubtype() byte {
	return (l.Type[1] >> 5) & 0x3
}

func (l *LSF) GNSS() *GNSS {
	if l.EncryptionType() == 0 && l.EncryptionSubtype() == 0x1 {
		g, err := NewGNSSFromMeta(l.Meta)
		if err != nil {
			log.Printf("[ERROR] Bad GNSS data: %v", err)
			return nil
		}
		return &g
	}
	return nil
}

func (l *LSF) ECD() *ECD {
	if l.EncryptionType() == 0 && l.EncryptionSubtype() == 0x2 {
		e := NewECDFromMeta(l.Meta)
		return &e
	}
	return nil
}

func (l *LSF) CAN() byte {
	return (l.Type[0] & 0x7)
}

func (l LSF) String() string {
	s := fmt.Sprintf(`{
	Dst: %s,
	Src: %s,
	Type: %#v,
	Meta: %#v,
	CRC: %#v,
	LSFType: %v,
	DataType: %v,
	EncryptionType: %v,
	EncryptionSubtype: %v
`,
		l.Dst.Callsign(),
		l.Src.Callsign(),
		l.Type,
		l.Meta,
		l.CRC,
		l.LSFType(),
		l.DataType(),
		l.EncryptionType(),
		l.EncryptionSubtype())
	if l.EncryptionType() == 0 {
		switch l.EncryptionSubtype() {
		case 0x0: // Text Data
			if l.Meta[0] != 0 {
				log.Printf("[DEBUG] Received LSF Text Data")
			}
		case 0x1: // GNSS Position Data
			log.Printf("[DEBUG] Received LSF GNSS Position Data")
			g := l.GNSS()
			if g != nil {
				s += g.String()
			}
		case 0x2: // Extended Callsign Data
			log.Printf("[DEBUG] Received LSF Extended Callsign Data")
			e := l.ECD()
			if e != nil {
				s += e.String()
			}
		}
	}
	s += "\n}"
	return s
}

type GNSS struct {
	DataSource     byte
	StationType    byte
	latDegrees     byte
	latFraction    uint16
	lonDegrees     byte
	lonFraction    uint16
	miscBits       byte
	altitude       uint16
	BearingDegrees uint16
	SpeedMPH       byte
}

func NewGNSS(dataSource, stationType byte, latitude, longitude float32, altitude, bearing, speed int, altitudeValid, speedBearingValid bool) GNSS {
	g := GNSS{
		DataSource:     dataSource,
		StationType:    stationType,
		altitude:       uint16(altitude + 1500),
		BearingDegrees: uint16(bearing),
		SpeedMPH:       byte(speed),
	}
	l, f := math.Modf(float64(latitude))
	if l < 0 {
		g.miscBits |= 0x1
		l = -l
	}
	g.latDegrees = byte(l)
	g.latFraction = uint16(f * 65535)
	l, f = math.Modf(float64(longitude))
	if l < 0 {
		g.miscBits |= 0x2
		l = -l
	}
	g.lonDegrees = byte(l)
	g.lonFraction = uint16(f * 65535)
	if altitudeValid {
		g.miscBits |= 0x4
	}
	if speedBearingValid {
		g.miscBits |= 0x8
	}
	return g
}
func NewGNSSFromMeta(meta [14]byte) (GNSS, error) {
	g := GNSS{
		DataSource:  meta[0],
		StationType: meta[1],
		latDegrees:  meta[2],
		// latFraction uint16
		lonDegrees: meta[5],
		// lonFraction    uint16
		miscBits: meta[8],
		// altitude       uint16
		// BearingDegrees uint16
		SpeedMPH: meta[13],
	}
	_, err := binary.Decode(meta[3:5], binary.BigEndian, &g.latFraction)
	if err != nil {
		return g, fmt.Errorf("decoding g.latFraction: %w", err)
	}
	_, err = binary.Decode(meta[6:8], binary.BigEndian, &g.lonFraction)
	if err != nil {
		return g, fmt.Errorf("decoding g.lonFraction: %w", err)
	}
	_, err = binary.Decode(meta[9:11], binary.BigEndian, &g.altitude)
	if err != nil {
		return g, fmt.Errorf("decoding g.altitude: %w", err)
	}
	_, err = binary.Decode(meta[11:13], binary.BigEndian, &g.BearingDegrees)
	if err != nil {
		return g, fmt.Errorf("decoding g.BearingDegrees: %w", err)
	}
	return g, nil
}
func (g *GNSS) Latitude() float32 {
	sign := float32(1.0)
	if g.miscBits&0x1 == 0x1 {
		sign = float32(-1.0)
	}
	return float32(g.latFraction)/65535 + float32(g.latDegrees)*sign
}
func (g *GNSS) Longitude() float32 {
	sign := float32(1.0)
	if g.miscBits&0x2 == 0x2 {
		sign = float32(-1.0)
	}
	return float32(g.lonFraction)/65535 + float32(g.lonDegrees)*sign
}

func (g *GNSS) ValidAltitude() bool {
	return g.miscBits&0x4 == 0x4
}
func (g *GNSS) Altitude() int {
	return int(g.altitude) - 1500
}
func (g *GNSS) ValidSpeedBearing() bool {
	return g.miscBits&0x8 == 0x8
}
func (g GNSS) String() string {
	s := fmt.Sprintf(`	GNSS: {
		DataSource:  %02x,
		StationType: %02x,
		Latitude:    %5.3f,
		Longitude:   %5.3f,`, g.DataSource, g.StationType, g.Latitude(), g.Longitude())
	if g.ValidAltitude() {
		s += fmt.Sprintf(`
		Altitude:    %d,`, g.Altitude())
	}
	if g.ValidSpeedBearing() {
		s += fmt.Sprintf(`
		Bearing:     %d,
		Speed:       %d,
	}`, g.BearingDegrees, g.SpeedMPH)
	}
	s += "\n	}"
	return s
}

type ECD struct {
	Callsign1 []byte
	Callsign2 []byte
}

func NewECDFromMeta(meta [14]byte) ECD {
	return ECD{
		Callsign1: meta[:6],
		Callsign2: meta[6:12],
	}
}

func (e ECD) String() string {
	cs1, err := DecodeCallsign(e.Callsign1)
	if err != nil {
		cs1 = err.Error()
	}
	cs2, err := DecodeCallsign(e.Callsign2)
	if err != nil {
		cs2 = err.Error()
	}
	s := fmt.Sprintf(`	ECD: {
		Callsign1: %s,
		Callsign2: %s
	}`, cs1, cs2)
	return s
}
