package gateway

import "github.com/jancona/m17/pkg/m17"

// ReceivedRFLSF updates gateway state when a radio stream begins.
func (s *Service) ReceivedRFLSF(lsf m17.LSF, _ float64) error {
	if s.State() != Idle || lsf.Type[1]&byte(m17.LSFTypeStream) == 0 {
		return nil
	}
	switch lsf.Dst.Callsign() {
	case "/ECHO", "#ECHO":
		s.SetState(Echo)
		s.echoStream = make([]m17.StreamDatagram, 0)
	case "/INFO", "#INFO":
		s.SetState(LocalCommand)
	default:
		s.SetState(RFStreamRX)
	}
	return nil
}

// ReceivedRFStreamFrame forwards a radio stream frame or stores it for echo.
func (s *Service) ReceivedRFStreamFrame(lsf m17.LSF, payload []byte, streamID, frameNumber uint16, _ float64) error {
	datagram := m17.NewStreamDatagram(streamID, frameNumber, &lsf, payload)
	switch s.State() {
	case Echo:
		s.echoStream = append(s.echoStream, datagram)
	case RFStreamRX:
		if s.dependencies.Reflector != nil {
			if err := s.dependencies.Reflector.SendStream(datagram); err != nil {
				return err
			}
		}
		if s.config.Duplex {
			datagram.LSF.SetECD(&datagram.LSF.Src, nil)
			datagram.LSF.Src = s.encodedCallsign
			return s.dependencies.Modem.TransmitVoiceStream(datagram)
		}
	}
	return nil
}

// ReceivedRFStreamLICH handles a reconstructed stream LSF.
func (s *Service) ReceivedRFStreamLICH(lsf m17.LSF, ber float64) error {
	if s.State() == Idle {
		return s.ReceivedRFLSF(lsf, ber)
	}
	return nil
}

// ReceivedRFStreamEOT ends an RF stream.
func (s *Service) ReceivedRFStreamEOT(_ m17.LSF, _ uint16, _ uint16, _ float64) error {
	switch s.State() {
	case Echo:
		go s.echoStreamEnd()
	case LocalCommand:
		go s.PlayMessage("welcome")
	case RFStreamRX:
		s.SetState(Idle)
	}
	return nil
}

// ReceivedRFPacket forwards packet traffic to the reflector.
func (s *Service) ReceivedRFPacket(lsf m17.LSF, payload []byte, _ float64) error {
	packet := m17.NewPacketFromBytes(append(lsf.ToBytes(), payload...))
	if s.dependencies.Reflector != nil {
		if err := s.dependencies.Reflector.SendPacket(packet); err != nil {
			return err
		}
	}
	if s.config.Duplex {
		packet.LSF.SetECD(&s.encodedCallsign, nil)
		return s.dependencies.Modem.TransmitPacket(packet)
	}
	return nil
}
