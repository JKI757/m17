package m17

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
)

const (
	samplesPerSecond = 24000

// samplesPer40MS   = samplesPerSecond / 1000 * 40
// samplesPerSymbol = 5
// symbolsPerSecond = samplesPerSecond / 5
// symbolsPer40MS   = symbolsPerSecond / 1000 * 40
)

type Modem interface {
	io.ReadCloser
	TransmitPacket(Packet) error
	TransmitVoiceStream(StreamDatagram) error
	Start() error
	Reset() error
	SetAFC(afc bool) error
	SetFreqCorrection(corr int16) error
	SetRXFreq(freq uint32) error
	SetTXFreq(freq uint32) error
	SetTXPower(dbm float32) error
}

type DummyModem struct {
	In    io.ReadCloser
	Out   io.WriteCloser
	extra []byte
}

func (m *DummyModem) TransmitPacket(p Packet) error {
	encoded, err := p.Encode()
	if err != nil {
		return err
	}
	err = binary.Write(m.Out, binary.LittleEndian, encoded)
	if err != nil {
		return fmt.Errorf("failed to send: %w", err)
	}
	return nil
}

func (m *DummyModem) TransmitVoiceStream(sd StreamDatagram) error {
	return nil
}

func (m *DummyModem) Read(p []byte) (n int, err error) {
	l := len(p)
	el := len(m.extra)
	log.Printf("[DEBUG] Request to read %d bytes, el: %d", l, el)
	if el < l {
		rl := (l - el) / 5
		if (l-el)%5 > 0 {
			// make sure we have enough symbols
			rl++
		}
		// Get whole symbols
		if rl%4 > 0 {
			rl += 4 - rl%4
		}
		sBuff := make([]byte, rl)
		log.Printf("[DEBUG] Attempting to read %d bytes", len(sBuff))
		nn, err := m.In.Read(sBuff)
		if err != nil {
			log.Printf("[ERROR] DummyModem Read failed: %v", err)
			if nn == 0 {
				return 0, err
			}
		}
		log.Printf("[DEBUG] Read %d bytes", nn)
		if nn%4 != 0 {
			// panic("handle this!")
			nn -= nn % 4
		}
		sBuff = sBuff[:nn]
		for i := 0; i < nn; i += 4 {
			// Repeat each read symbol 5 times
			for range 5 {
				m.extra = append(m.extra, sBuff[i:i+4]...)
			}
		}
		l = min(l, len(m.extra))
	}
	n = copy(p, m.extra[:l])
	m.extra = m.extra[l:]
	log.Printf("[DEBUG] Returning %d bytes", n)
	return
}

func (m *DummyModem) Write(buf []byte) (n int, err error) {
	return m.Out.Write(buf)
}
func (m *DummyModem) Reset() error {
	return nil
}
func (m *DummyModem) SetAFC(afc bool) error {
	return nil
}
func (m *DummyModem) SetFreqCorrection(corr int16) error {
	return nil
}
func (m *DummyModem) SetRXFreq(freq uint32) error {
	return nil
}
func (m *DummyModem) SetTXFreq(freq uint32) error {
	return nil
}
func (m *DummyModem) SetTXPower(dbm float32) error {
	return nil
}
func (m *DummyModem) Start() error {
	return nil
}

func (m *DummyModem) Close() error {
	err := m.In.Close()
	err2 := m.Out.Close()
	return errors.Join(err, err2)
}
