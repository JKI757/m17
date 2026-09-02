package m17

import "github.com/jancona/m17/pkg/protocol"

type (
	EncodedCallsign   = protocol.EncodedCallsign
	LSF               = protocol.LSF
	LSFType           = protocol.LSFType
	LSFDataType       = protocol.LSFDataType
	LSFEncryptionType = protocol.LSFEncryptionType
	GNSS              = protocol.GNSS
	ECD               = protocol.ECD
	Packet            = protocol.Packet
	PacketType        = protocol.PacketType
)

const (
	EncodedCallsignLen         = protocol.EncodedCallsignLen
	MaxCallsignLen             = protocol.MaxCallsignLen
	DestinationAll             = protocol.DestinationAll
	EncodedDestinationAll      = protocol.EncodedDestinationAll
	MaxEncodedCallsign         = protocol.MaxEncodedCallsign
	SpecialEncodedRange        = protocol.SpecialEncodedRange
	CRCLen                     = protocol.CRCLen
	LSFTypePacket              = protocol.LSFTypePacket
	LSFTypeStream              = protocol.LSFTypeStream
	LSFDataTypeReserved        = protocol.LSFDataTypeReserved
	LSFDataTypeData            = protocol.LSFDataTypeData
	LSFDataTypeVoice           = protocol.LSFDataTypeVoice
	LSFDataTypeVoiceData       = protocol.LSFDataTypeVoiceData
	LSFEncryptionTypeNone      = protocol.LSFEncryptionTypeNone
	LSFEncryptionTypeScrambler = protocol.LSFEncryptionTypeScrambler
	LSFEncryptionTypeAES       = protocol.LSFEncryptionTypeAES
	LSFEncryptionTypeOther     = protocol.LSFEncryptionTypeOther
	LSFLen                     = protocol.LSFLen
	LSDLen                     = protocol.LSDLen
	PacketTypeRAW              = protocol.PacketTypeRAW
	PacketTypeAX25             = protocol.PacketTypeAX25
	PacketTypeAPRS             = protocol.PacketTypeAPRS
	PacketType6LoWPAN          = protocol.PacketType6LoWPAN
	PacketTypeIPv4             = protocol.PacketTypeIPv4
	PacketTypeSMS              = protocol.PacketTypeSMS
	PacketTypeWinlink          = protocol.PacketTypeWinlink
)

var (
	EncodedDestinationAllBytes = protocol.EncodedDestinationAllBytes
	CallsignRegex              = protocol.CallsignRegex
)

var (
	EncodeCallsign          = protocol.EncodeCallsign
	DecodeCallsign          = protocol.DecodeCallsign
	NormalizeCallsignModule = protocol.NormalizeCallsignModule
	NewEmptyLSF             = protocol.NewEmptyLSF
	NewLSF                  = protocol.NewLSF
	NewLSFFromBytes         = protocol.NewLSFFromBytes
	NewLSFFromLSD           = protocol.NewLSFFromLSD
	NewGNSSFromMeta         = protocol.NewGNSSFromMeta
	NewECDFromMeta          = protocol.NewECDFromMeta
	NewPacketFromBytes      = protocol.NewPacketFromBytes
	NewPacket               = protocol.NewPacket
	CRC                     = protocol.CRC
)
