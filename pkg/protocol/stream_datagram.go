package protocol

import (
    "encoding/binary"
    "fmt"
    "log"
    fecpkg "github.com/jancona/m17/pkg/fec"
)

const magicM17Voice = "M17 "

type StreamDatagram struct {
    StreamID    uint16
    FrameNumber uint16
    LastFrame   bool
    LSF         LSF
    Payload     [16]byte
}

func NewStreamDatagramFromBytes(buffer []byte) (StreamDatagram, error) {
    sd := StreamDatagram{}
    if len(buffer) != 54 {
        return sd, fmt.Errorf("stream datagram buffer length %d != 50", len(buffer))
    }
    if fecpkg.CRC(buffer) != 0 {
        return sd, fmt.Errorf("bad CRC for stream datagram buffer")
    }
    buffer = buffer[4:]
    _, err := binary.Decode(buffer, binary.BigEndian, &sd.StreamID)
    if err != nil {
        log.Printf("[INFO] Unable to decode streamID from stream datagram: %v", err)
        return sd, fmt.Errorf("bad streamID from stream datagram: %v", err)
    }
    sd.LSF = NewLSFFromLSD(buffer[2:30])
    sd.LSF.CalcCRC()

    _, err = binary.Decode(buffer[30:], binary.BigEndian, &sd.FrameNumber)
    if err != nil {
        log.Printf("[INFO] Unable to decode frameNumber from stream datagram: %v", err)
        return sd, fmt.Errorf("bad frameNumber from stream datagram: %v", err)
    }
    sd.LastFrame = sd.FrameNumber&0x8000 == 0x8000
    // sd.FrameNumber &= 0x7fff
    copy(sd.Payload[:], buffer[32:48])
    return sd, nil
}

func NewStreamDatagram(streamID uint16, frameNumber uint16, lsf LSF, payload []byte) StreamDatagram {
    sd := StreamDatagram{
        StreamID:    streamID,
        FrameNumber: frameNumber,
        LastFrame:   frameNumber&0x8000 == 0x8000,
        LSF:         lsf,
    }
    copy(sd.Payload[:], payload)
    return sd
}

func (sd StreamDatagram) ToBytes() []byte {
    buf := make([]byte, 0, 54)
    buf = append(buf, []byte(magicM17Voice)...)
    buf, _ = binary.Append(buf, binary.BigEndian, sd.StreamID)
    buf = append(buf, sd.LSF.ToLSDBytes()...)
    buf, _ = binary.Append(buf, binary.BigEndian, sd.FrameNumber)
    buf = append(buf, sd.Payload[:]...)
    crc := fecpkg.CRC(buf[:52])
    buf, _ = binary.Append(buf, binary.BigEndian, crc)
    return buf
}

func (sd StreamDatagram) String() string {
    return fmt.Sprintf(`{
    StreamID: %04x,
    FrameNumber: %04x,
    LastFrame: %v,
    LSF: %s,
    Payload: [% 2x],
}`, sd.StreamID, sd.FrameNumber, sd.LastFrame, sd.LSF, sd.Payload)
}

