package m17

import "github.com/jancona/m17/pkg/inet"

type (
	InetClient     = inet.InetClient
	StreamDatagram = inet.StreamDatagram
)

const (
	MagicLen       = inet.MagicLen
	MagicACKN      = inet.MagicACKN
	MagicCONN      = inet.MagicCONN
	MagicDISC      = inet.MagicDISC
	MagicLSTN      = inet.MagicLSTN
	MagicNACK      = inet.MagicNACK
	MagicPING      = inet.MagicPING
	MagicPONG      = inet.MagicPONG
	MagicM17Stream = inet.MagicM17Stream
	MagicM17Packet = inet.MagicM17Packet
)

var (
	NewInetClient              = inet.NewInetClient
	NewStreamDatagram          = inet.NewStreamDatagram
	NewStreamDatagramFromBytes = inet.NewStreamDatagramFromBytes
)
