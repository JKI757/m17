package phy

import (
    "fmt"
    protocol "github.com/jancona/m17/pkg/protocol"
)

// EncodePacket converts a protocol.Packet into the symbol stream for transmission.
func EncodePacket(p *protocol.Packet) ([]Symbol, error) {
    outPacket := make([]Symbol, 0, 36*192*10) // full packet: up to 36 frames
    b, err := ConvolutionalEncode(p.LSF.ToBytes(), LSFPuncturePattern, LSFFinalBit)
    if err != nil {
        return nil, fmt.Errorf("unable to encode LSF: %w", err)
    }
    encodedBits := NewBits(b)
    // Preamble
    outPacket = AppendPreamble(outPacket, LSFPreamble)
    // LSF syncword
    outPacket = AppendSyncword(outPacket, LSFSync)
    rfBits := InterleaveBits(encodedBits)
    rfBits = RandomizeBits(rfBits)
    outPacket = AppendBits(outPacket, rfBits)

    chunkCnt := 0
    packetData := p.PayloadBytes()
    for bytesLeft := len(packetData); bytesLeft > 0; bytesLeft -= 25 {
        outPacket = AppendSyncword(outPacket, PacketSync)
        chunk := make([]byte, 26) // 25 bytes + 6 bits metadata
        if bytesLeft > 25 {
            copy(chunk, packetData[chunkCnt*25:chunkCnt*25+25])
            chunk[25] = byte(chunkCnt << 2)
        } else {
            copy(chunk, packetData[chunkCnt*25:chunkCnt*25+bytesLeft])
            if bytesLeft%25 == 0 {
                chunk[25] = (1 << 7) | ((25) << 2)
            } else {
                chunk[25] = uint8((1 << 7) | ((bytesLeft % 25) << 2))
            }
        }
        b, err := ConvolutionalEncode(chunk, PacketPuncturePattern, PacketModeFinalBit)
        if err != nil {
            return nil, fmt.Errorf("unable to encode packet: %w", err)
        }
        encodedBits := NewBits(b)
        rfBits := InterleaveBits(encodedBits)
        rfBits = RandomizeBits(rfBits)
        outPacket = AppendBits(outPacket, rfBits)
        chunkCnt++
    }
    outPacket = AppendEOT(outPacket)
    return outPacket, nil
}
