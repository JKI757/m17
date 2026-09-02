package gateway

import "github.com/jancona/m17/pkg/m17"

// ReceivePacket transmits a packet received from the Internet reflector.
func (s *Service) ReceivePacket(packet m17.Packet) error {
	packet.LSF.SetECD(&s.encodedCallsign, nil)
	return s.dependencies.Modem.TransmitPacket(packet)
}

// ReceiveStream transmits a stream frame received from the Internet reflector.
func (s *Service) ReceiveStream(datagram m17.StreamDatagram) error {
	datagram.LSF.SetECD(&s.encodedCallsign, nil)
	datagram.LSF.Src = s.encodedCallsign
	datagram.LSF.CalcCRC()
	return s.dependencies.Modem.TransmitVoiceStream(datagram)
}
