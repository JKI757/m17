package gateway

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jancona/m17/pkg/m17"
)

var silence3200 = []byte{0x01, 0x00, 0x09, 0x43, 0x9C, 0xE4, 0x21, 0x08, 0x01, 0x00, 0x09, 0x43, 0x9C, 0xE4, 0x21, 0x08}

// LoadAudioClips loads named Codec2 audio clips.
func (s *Service) LoadAudioClips(dir string) error {
	s.audioClips = map[string][]byte{}
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read audio clips: %w", err)
	}
	for _, file := range files {
		if !file.Type().IsRegular() || !strings.HasSuffix(file.Name(), ".dat") || file.Name() == "speak.dat" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			return fmt.Errorf("read audio clip %s: %w", file.Name(), err)
		}
		s.audioClips[strings.TrimSuffix(file.Name(), ".dat")] = padClip(data)
	}
	return nil
}

func padClip(data []byte) []byte {
	if rem := len(data) % len(silence3200); rem != 0 {
		data = append(data, silence3200[:len(silence3200)-rem]...)
	}
	return data
}

// PlayMessage sends named audio clips over RF.
func (s *Service) PlayMessage(words ...string) error {
	defer s.SetState(Idle)
	lsf, err := m17.NewLSF(m17.DestinationAll, s.config.Callsign, m17.LSFTypeStream, m17.LSFDataTypeVoice, 0)
	if err != nil {
		return err
	}
	streamID := uint16(rand.Intn(1 << 16))
	frameNumber := uint16(0)
	for _, word := range words {
		clip := s.audioClips[strings.ToLower(word)]
		for offset := 0; offset < len(clip); offset += 16 {
			datagram := m17.NewStreamDatagram(streamID, frameNumber, &lsf, clip[offset:offset+16])
			if err := s.dependencies.Modem.TransmitVoiceStream(datagram); err != nil {
				return err
			}
			frameNumber++
		}
	}
	frameNumber |= 0x8000
	datagram := m17.NewStreamDatagram(streamID, frameNumber, &lsf, silence3200)
	if err := s.dependencies.Modem.TransmitVoiceStream(datagram); err != nil {
		return err
	}
	time.Sleep(m17.FrameTime)
	return nil
}

func (s *Service) echoStreamEnd() error {
	defer s.SetState(Idle)
	streamID := uint16(rand.Intn(1 << 16))
	for _, datagram := range s.echoStream {
		datagram.StreamID = streamID
		datagram.LSF.Dst = datagram.LSF.Src
		datagram.LSF.Src = s.encodedCallsign
		datagram.LSF.CalcCRC()
		if err := s.dependencies.Modem.TransmitVoiceStream(datagram); err != nil {
			return err
		}
	}
	s.echoStream = nil
	return nil
}
