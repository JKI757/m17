package m17

const (
	samplesPerSecond = 24000

// samplesPer40MS   = samplesPerSecond / 1000 * 40
// samplesPerSymbol = 5
// symbolsPerSecond = samplesPerSecond / 5
// symbolsPer40MS   = symbolsPerSecond / 1000 * 40
)

type Modem interface {
	StartDecoding(sink func(typ uint16, softBits []SoftBit))
	Start() error
	Reset() error
	Close() error
	SetAFC(afc bool) error
	SetFreqCorrection(corr int16) error
	SetRXFreq(freq uint32) error
	SetTXFreq(freq uint32) error
	SetTXPower(dbm float32) error
	TransmitPacket(Packet) error
	TransmitVoiceStream(StreamDatagram) error
}
