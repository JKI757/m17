package m17

import "github.com/jancona/m17/pkg/codec"

type (
	Symbol          = codec.Symbol
	SoftBit         = codec.SoftBit
	Preamble        = codec.Preamble
	Bit             = codec.Bit
	PayloadBits     = codec.PayloadBits
	PuncturePattern = codec.PuncturePattern
	ViterbiDecoder  = codec.ViterbiDecoder
)

const (
	SymbolsPerSyncword = codec.SymbolsPerSyncword
	SymbolsPerPayload  = codec.SymbolsPerPayload
	SymbolsPerFrame    = codec.SymbolsPerFrame
	BytesPerFrame      = codec.BytesPerFrame
	BitsPerSymbol      = codec.BitsPerSymbol
	BitsPerPayload     = codec.BitsPerPayload
	FrameTime          = codec.FrameTime
	FramesPerSecond    = codec.FramesPerSecond
	PacketModeFinalBit = codec.PacketModeFinalBit
	LSFFinalBit        = codec.LSFFinalBit
	ConvolutionK       = codec.ConvolutionK
	ConvolutionStates  = codec.ConvolutionStates
	LSFPreamble        = codec.LSFPreamble
	BERTPreamble       = codec.BERTPreamble
	SoftZero           = codec.SoftZero
	SoftOne            = codec.SoftOne
	SoftErasure        = codec.SoftErasure
	SoftThreshold      = codec.SoftThreshold
)

var (
	SymbolMap                    = codec.SymbolMap
	SymbolList                   = codec.SymbolList
	EOTSymbols                   = codec.EOTSymbols
	LSFPuncturePattern           = codec.LSFPuncturePattern
	StreamPuncturePattern        = codec.StreamPuncturePattern
	PacketPuncturePattern        = codec.PacketPuncturePattern
	LSFPreambleSymbols           = codec.LSFPreambleSymbols
	LSFSyncSymbols               = codec.LSFSyncSymbols
	ExtLSFSyncSymbols            = codec.ExtLSFSyncSymbols
	StreamSyncSymbols            = codec.StreamSyncSymbols
	PacketSyncSymbols            = codec.PacketSyncSymbols
	BERTSyncSymbols              = codec.BERTSyncSymbols
	EOTMarkerSymbols             = codec.EOTMarkerSymbols
	NewPayloadBits               = codec.NewPayloadBits
	EuclNorm                     = codec.EuclNorm
	AppendPreamble               = codec.AppendPreamble
	AppendSyncwordSymbols        = codec.AppendSyncwordSymbols
	InterleaveBits               = codec.InterleaveBits
	DeinterleaveSoftBits         = codec.DeinterleaveSoftBits
	RandomizeBits                = codec.RandomizeBits
	DerandomizeSoftBits          = codec.DerandomizeSoftBits
	AppendBits                   = codec.AppendBits
	AppendEOT                    = codec.AppendEOT
	ConvolutionalEncode          = codec.ConvolutionalEncode
	ConvolutionalEncodeStream    = codec.ConvolutionalEncodeStream
	Encode24                     = codec.Encode24
	SoftBitXOR                   = codec.SoftBitXOR
	SoftXOR                      = codec.SoftXOR
	IntToSoft                    = codec.IntToSoft
	SoftToInt                    = codec.SoftToInt
	SoftPopCount                 = codec.SoftPopCount
	SoftCalcChecksum             = codec.SoftCalcChecksum
	SoftDetectErrors             = codec.SoftDetectErrors
	SoftDecode24                 = codec.SoftDecode24
	DecodeLICH                   = codec.DecodeLICH
	EncodeLICH                   = codec.EncodeLICH
	HardDecode24                 = codec.HardDecode24
	CalculateHammingDistance     = codec.CalculateHammingDistance
	IsValidCodeword              = codec.IsValidCodeword
	CalculateSyndrome            = codec.CalculateSyndrome
	GetErrorCorrectionCapability = codec.GetErrorCorrectionCapability
	GetMinimumDistance           = codec.GetMinimumDistance
)

const (
	softTrue    = SoftOne
	softMaybe   = softTrue / 2
	softFalse   = SoftZero
	lsfPreamble = LSFPreamble
)

func syncDistance(symbols []Symbol, offset int, sps int) (float32, uint16) {
	return codec.SyncDistance(symbols, offset, sps)
}
