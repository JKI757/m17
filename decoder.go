package m17

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"time"
)

const (
	LSFSync    = uint16(0x55F7)
	StreamSync = uint16(0xFF5D)
	PacketSync = uint16(0x75FF)
	BERTSync   = uint16(0xDF55)
	EOTMarker  = uint16(0x555D)
)

var (
	LSFSyncBytes    = []byte{0x55, 0xF7}
	StreamSyncBytes = []byte{0xFF, 0x5D}
	PacketSyncBytes = []byte{0x75, 0xFF}
	BERTSyncBytes   = []byte{0xDF, 0x55}
	EOTMarkerBytes  = []byte{0x55, 0x5D}
)

var emptyFrameData = []byte{
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

type Decoder struct {
	sendToNetwork func(lsf *LSF, payload []byte, sid, fn uint16) error
	syncedType    uint16

	lsf *LSF

	frameData  []byte //decoded frame data, 206 bits, plus 4 flushing bits
	packetData []byte //whole packet data

	timeoutCnt   int
	gotLSF       bool
	lastPacketFN byte   // last packet frame number received (0xff when idle)
	lastStreamFN uint16 // last stream frame number received (0xffff when idle)
	lichParts    int
	streamID     uint16
	streamFN     uint16
	lsfBytes     []byte
	dashLog      *slog.Logger
	lastLogTime  time.Time
	errors       int
	bits         int
}

// 8 preamble symbols, 8 for the syncword, and 960 for the payload.
// floor(sps/2)=2 extra samples for timing error correction
// plus some extra so we can make larger reads
const symbolBufSize = 8*5 + 2*(8*5+4800/25*5) + 2 + 256

func NewDecoder(dashLog *slog.Logger, sendToNetwork func(lsf *LSF, payload []byte, sid, fn uint16) error, duplex bool) *Decoder {
	d := Decoder{
		sendToNetwork: sendToNetwork,
		lastPacketFN:  0xff,
		lastStreamFN:  0xffff,
		lsfBytes:      make([]byte, 30),
		dashLog:       dashLog,
	}
	return &d
}

func (d *Decoder) DecodeFrame(typ uint16, softBits []SoftBit) {
	switch {
	case typ == LSFSync && d.syncedType == 0:
		d.gotLSF = false
		var e int
		d.lsf, e = decodeLSF(softBits)
		if d.lsf.CheckCRC() {
			log.Printf("[DEBUG] Received RF LSF: %s", d.lsf)
			d.gotLSF = true
			d.timeoutCnt = 0
			d.lastStreamFN = 0xffff
			d.lastPacketFN = 0xff
			d.errors = e
			d.bits = 368

			if d.lsf.Type[1]&byte(LSFTypeStream) == byte(LSFTypeStream) {
				d.syncedType = StreamSync
				d.lichParts = 0
				d.streamFN = 0
				d.streamID = uint16(rand.Intn(0x10000))
				// d.sendToNetwork(d.lsf, nil, d.streamID, d.streamFN)
				if d.dashLog != nil {
					d.dashLog.Info("", "type", "RF", "subtype", "Voice Start", "src", d.lsf.Src.Callsign(), "dst", d.lsf.Dst.Callsign(), "can", d.lsf.CAN(), "mer", json.Number(fmt.Sprintf("%f", float64(e)/3.68)))
					gnss := d.lsf.GNSS()
					if gnss != nil && gnss.ValidLatLon {
						d.lastLogTime = time.Now()
						args := []any{
							"type", "RF",
							"subtype", "GNSS",
							"src", d.lsf.Src.Callsign(),
							"dataSource", gnss.DataSource,
							"stationType", gnss.StationType,
							"latitude", json.Number(fmt.Sprintf("%f", gnss.Latitude)),
							"longitude", json.Number(fmt.Sprintf("%f", gnss.Longitude)),
						}
						if gnss.ValidAltitude {
							args = append(args,
								"altitude", json.Number(fmt.Sprintf("%.1f", gnss.Altitude)),
							)
						}
						if gnss.ValidBearingSpeed {
							args = append(args,
								"speed", json.Number(fmt.Sprintf("%.1f", gnss.Speed)),
								"bearing", gnss.Bearing,
							)
						}
						if gnss.ValidRadius {
							args = append(args,
								"radius", gnss.Radius,
							)
						}
						d.dashLog.Info("", args...)
					}
				}
			} else { // packet mode
				d.syncedType = PacketSync
				d.packetData = make([]byte, 33*25)
			}
		} else {
			log.Print("[DEBUG] Bad LSF CRC")
		}

	case typ == PacketSync && d.syncedType == PacketSync:
		pktFrame, e := d.decodePacketFrame(softBits)
		d.errors += e
		d.bits += 368
		// log.Printf("[DEBUG] pktFrame: % x", pktFrame)
		lastFrame := (pktFrame[25] >> 7) != 0

		// If lastFrame is true, this value is the byte count in the frame,
		// otherwise it's the frame number
		frameNumOrByteCnt := byte((pktFrame[25] >> 2) & 0x1F)

		if lastFrame && frameNumOrByteCnt > 25 {
			log.Printf("[INFO] Fixing overrun in last frame: %d > 25", frameNumOrByteCnt)
			frameNumOrByteCnt = 25
		}

		log.Printf("[DEBUG] pktFrame[25]: %b, frameNumOrByteCnt: %d, last: %v", pktFrame[25], frameNumOrByteCnt, lastFrame)
		if lastFrame {
			log.Printf("[DEBUG] Frame %d BER: %1.1f", d.lastPacketFN+1, float64(e)/3.68)
		} else {
			log.Printf("[DEBUG] Frame %d BER: %1.1f", frameNumOrByteCnt, float64(e)/3.68)
		}
		// log.Printf("[DEBUG] frameData: % x %s", pktFrame, pktFrame)

		//copy data - might require some fixing
		if frameNumOrByteCnt <= 31 && frameNumOrByteCnt == d.lastPacketFN+1 && !lastFrame {
			copy(d.packetData[frameNumOrByteCnt*25:(frameNumOrByteCnt+1)*25], pktFrame)
			d.lastPacketFN++
		} else if lastFrame {
			copy(d.packetData[(d.lastPacketFN+1)*25:(d.lastPacketFN+1)*25+frameNumOrByteCnt], pktFrame[:frameNumOrByteCnt])
			d.packetData = d.packetData[:(d.lastPacketFN+1)*25+frameNumOrByteCnt]
			log.Printf("[DEBUG] pktFrame[:frameNumOrByteCnt]: % 0x, d.packetData: % 0x", pktFrame[:frameNumOrByteCnt], d.packetData)
			if CRC(d.packetData) == 0 {
				// log.Printf("[DEBUG] d.lsf: %v, d.packetData: %v", d.lsf, d.packetData)
				d.sendToNetwork(d.lsf, d.packetData, 0, 0)
				p := NewPacketFromBytes(append(d.lsf.ToBytes(), d.packetData...))
				if d.dashLog != nil {
					if p.Type == PacketTypeSMS && len(p.Payload) > 0 {
						msg := string(p.Payload[0 : len(p.Payload)-1])
						d.dashLog.Info("", "type", "RF", "subtype", "Packet", "src", d.lsf.Src.Callsign(), "dst", d.lsf.Dst.Callsign(), "can", d.lsf.CAN(), "mer", json.Number(fmt.Sprintf("%f", float64(d.errors)/float64(d.bits)*100)), "packetType", p.Type, "smsMessage", msg)
					} else {
						d.dashLog.Info("", "type", "RF", "subtype", "Packet", "src", d.lsf.Src.Callsign(), "dst", d.lsf.Dst.Callsign(), "can", d.lsf.CAN(), "mer", json.Number(fmt.Sprintf("%f", float64(d.errors)/float64(d.bits)*100)), "packetType", p.Type)
					}
				}
			} else {
				log.Printf("[DEBUG] Bad CRC not forwarded: %x", CRC(d.packetData))
			}
			// cleanup
			d.reset()
		}

	case typ == StreamSync:
		var lich []byte
		var lichCnt byte
		var e int
		var fn uint16
		d.frameData, lich, fn, lichCnt, e = d.decodeStreamFrame(softBits)
		log.Printf("[DEBUG] frameData: [% 2x], lich: %02x, lichCnt: %d, d.lichParts: %04x, fn: %04x, d.lastStreamFN: %04x, e: %d", d.frameData, lich, lichCnt, d.lichParts, fn, d.lastStreamFN, e)
		d.errors += e
		d.bits += 272
		if d.lastStreamFN+1 == fn&0x7fff {
			if d.lichParts != 0x3F && lichCnt < 6 { //6 chunks = 0b111111
				//reconstruct LSF chunk by chunk
				copy(d.lsfBytes[lichCnt*5:lichCnt*5+5], lich)
				d.lichParts |= (1 << lichCnt)
				if d.lichParts == 0x3F {
					d.lichParts = 0
					lsfB := NewLSFFromBytes(d.lsfBytes)
					if lsfB.CheckCRC() {
						d.lsf = lsfB
						d.gotLSF = true
						d.timeoutCnt = 0
						log.Printf("[DEBUG] Received stream LSF: %v", lsfB)
						gnss := d.lsf.GNSS()
						if d.dashLog != nil &&
							gnss != nil &&
							gnss.ValidLatLon &&
							time.Since(d.lastLogTime) > 15*time.Second {
							d.lastLogTime = time.Now()
							args := []any{
								"type", "RF",
								"subtype", "GNSS",
								"dataSource", gnss.DataSource,
								"stationType", gnss.StationType,
								"src", d.lsf.Src.Callsign(),
								"latitude", json.Number(fmt.Sprintf("%f", gnss.Latitude)),
								"longitude", json.Number(fmt.Sprintf("%f", gnss.Longitude)),
							}
							if gnss.ValidAltitude {
								args = append(args,
									"altitude", json.Number(fmt.Sprintf("%.1f", gnss.Altitude)),
								)
							}
							if gnss.ValidBearingSpeed {
								args = append(args,
									"speed", json.Number(fmt.Sprintf("%.1f", gnss.Speed)),
									"bearing", gnss.Bearing,
								)
							}
							if gnss.ValidRadius {
								args = append(args,
									"radius", gnss.Radius,
								)
							}
							d.dashLog.Info("", args...)
						}
					} else {
						log.Printf("[DEBUG] Stream LSF CRC error: %v", lsfB)
						d.gotLSF = false
					}
				}
			}
			log.Printf("[DEBUG] Received stream frame: FN:%04X, LICH_CNT:%d, e: %d, BER: %1.1f", fn, lichCnt, e, float64(e)/2.72)
			lastFrame := fn&0x8000 == 0x8000
			if d.gotLSF {
				d.streamFN = fn
				d.sendToNetwork(d.lsf, d.frameData, d.streamID, d.streamFN)
				d.timeoutCnt = 0
				if d.dashLog != nil && lastFrame {
					log.Printf("[DEBUG] Last frame for RF voice stream %04x", d.streamID)
					d.dashLog.Info("", "type", "RF", "subtype", "Voice End", "src", d.lsf.Src.Callsign(), "dst", d.lsf.Dst.Callsign(), "can", d.lsf.CAN(), "mer", json.Number(fmt.Sprintf("%f", float64(d.errors)/float64(d.bits)*100)))
				}
			}
			if lastFrame {
				d.reset()
			} else {
				d.lastStreamFN = fn
			}
		}
	case typ == EOTMarker && d.syncedType == StreamSync:
		if d.gotLSF {
			d.streamFN = uint16(d.lastStreamFN+1) | 0x8000
			d.sendToNetwork(d.lsf, emptyFrameData, d.streamID, d.streamFN)
			if d.dashLog != nil {
				log.Printf("[DEBUG] EOT for RF voice stream %04x", d.streamID)
				d.dashLog.Info("", "type", "RF", "subtype", "Voice End", "src", d.lsf.Src.Callsign(), "dst", d.lsf.Dst.Callsign(), "can", d.lsf.CAN(), "mer", 0)
			}
		}
		// reset
		d.reset()
	}
	//RX sync timeout
	if d.syncedType != 0 {
		d.timeoutCnt++
		if d.timeoutCnt > 960*2 {
			if d.dashLog != nil && d.gotLSF && d.lastStreamFN&0x8000 != 0x8000 {
				// If we timed out of a voice stream without a last frame, send the Voice End here
				log.Printf("[DEBUG] Timed out RF voice stream %04x", d.streamID)
				d.dashLog.Info("", "type", "RF", "subtype", "Voice End", "src", d.lsf.Src.Callsign(), "dst", d.lsf.Dst.Callsign(), "can", d.lsf.CAN(), "mer", 0)
			}
			d.reset()
		}
	}
}

func decodeLSF(softBit []SoftBit) (*LSF, int) {
	// log.Printf("[DEBUG] decodeLSF: len(pld): %d", len(pld))
	// log.Printf("[DEBUG] softBit: %#v", softBit)

	//derandomize
	softBit = DerandomizeSoftBits(softBit)
	// log.Printf("[DEBUG] derandomized softBit: %#v", softBit)

	//deinterleave
	dSoftBit := DeinterleaveSoftBits(softBit)
	// log.Printf("[DEBUG] dSoftBit: %#v", dSoftBit)

	//decode
	vd := ViterbiDecoder{}
	lsf, e := vd.DecodePunctured(dSoftBit, LSFPuncturePattern)
	e = e - len(LSFPuncturePattern) + 1

	//shift the buffer 1 position left - get rid of the encoded flushing bits
	// copy(lsf, lsf[1:])
	lsf = lsf[1 : LSFLen+1]
	// log.Printf("[DEBUG] lsf: %x", lsf)
	if CRC(lsf) != 0 {
		log.Printf("[DEBUG] Bad LSF CRC: %x", CRC(lsf))
	} else {
		dst, err := DecodeCallsign(lsf[0:6])
		if err != nil {
			log.Printf("[ERROR] Bad dst callsign: %v", err)
		}
		src, err := DecodeCallsign(lsf[6:12])
		if err != nil {
			log.Printf("[ERROR] Bad src callsign: %v", err)
		}
		log.Printf("[DEBUG] dest: %s, src: %s", dst, src)
	}
	log.Printf("[DEBUG] LSF BER: %1.1f", float64(e)/3.68)
	l := NewLSFFromBytes(lsf)
	return l, e
}

func (d *Decoder) decodeStreamFrame(softBit []SoftBit) (frameData []byte, lich []byte, fn uint16, lichCnt byte, e int) {
	// log.Printf("[DEBUG] decodeStreamFrame: len(pld): %d", len(pld))
	// log.Printf("[DEBUG] pld: [% 1.1f]", pld)

	// softBit := calcSoftbits(pld)
	// log.Printf("[DEBUG] softBit: [% 04x]", softBit)

	//derandomize
	softBit = DerandomizeSoftBits(softBit)
	// log.Printf("[DEBUG] derandomized softBit: [% 04x]", softBit)

	//deinterleave
	dSoftBit := DeinterleaveSoftBits(softBit)
	// log.Printf("[DEBUG] deinterleaved softBit: [% 04x]", dSoftBit)
	lich = DecodeLICH(dSoftBit[:96])
	lichCnt = lich[5] >> 5

	//decode
	vd := ViterbiDecoder{}
	frameData, e = vd.DecodePunctured(dSoftBit[96:], StreamPuncturePattern)
	e = e - len(StreamPuncturePattern)

	// log.Printf("[DEBUG] frameData[:3]: [% 02x]", frameData[:3])
	fn = (uint16(frameData[1]) << 8) | uint16(frameData[2])

	//shift 1+2 positions left - get rid of the encoded flushing bits and FN
	frameData = frameData[1+2:]

	return frameData, lich, fn, lichCnt, e
}

func (d *Decoder) decodePacketFrame(softBit []SoftBit) ([]byte, int) {
	// log.Printf("[DEBUG] decodePacketFrame: len(pld): %d", len(pld))
	// log.Printf("[DEBUG] pld: %#v", pld)

	// softBit := calcSoftbits(pld)
	// log.Printf("[DEBUG] softBit: %#v", softBit)

	//derandomize
	softBit = DerandomizeSoftBits(softBit)
	// log.Printf("[DEBUG] derandomized softBit: %#v", softBit)

	//deinterleave
	dSoftBit := DeinterleaveSoftBits(softBit)
	// log.Printf("[DEBUG] dSoftBit: %#v", dSoftBit)

	//decode
	vd := ViterbiDecoder{}
	pkt, e := vd.DecodePunctured(dSoftBit, PacketPuncturePattern)
	// log.Printf("[DEBUG] pkt: %#v", pkt)
	e = e - len(PacketPuncturePattern)

	return pkt[1:], e
}

func calcSoftbits(pld []Symbol) []SoftBit {
	if len(pld) > SymbolsPerPayload {
		panic(fmt.Sprintf("pld contains %d symbols (>%d)", len(pld), SymbolsPerPayload))
	}
	softBit := make([]SoftBit, 2*SymbolsPerPayload) //raw frame soft bits

	for i, sym := range pld {

		//bit 0
		if sym >= SymbolList[3] {
			softBit[i*2+1] = softTrue
		} else if sym >= SymbolList[2] {
			softBit[i*2+1] = SoftBit(-softTrue/((SymbolList[3]-SymbolList[2])*SymbolList[2]) + sym*softTrue/(SymbolList[3]-SymbolList[2]))
		} else if sym >= SymbolList[1] {
			softBit[i*2+1] = softFalse
		} else if sym >= SymbolList[0] {
			softBit[i*2+1] = SoftBit(softTrue/((SymbolList[1]-SymbolList[0])*SymbolList[1]) - sym*softTrue/(SymbolList[1]-SymbolList[0]))
		} else {
			softBit[i*2+1] = softTrue
		}

		//bit 1
		if sym >= SymbolList[2] {
			softBit[i*2] = softFalse
		} else if sym >= SymbolList[1] {
			softBit[i*2] = SoftBit(softMaybe - (sym * softTrue / (SymbolList[2] - SymbolList[1])))
		} else {
			softBit[i*2] = softTrue
		}
	}
	return softBit
}

func (d *Decoder) reset() {
	d.syncedType = 0
	d.lsf = nil
	d.gotLSF = false
	d.timeoutCnt = 0
	d.lastPacketFN = 0xff
	d.lastStreamFN = 0xffff
	d.lichParts = 0
	d.errors = 0
	d.bits = 0
}
