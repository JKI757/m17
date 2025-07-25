package m17

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"go.bug.st/serial"
)

// CC1200 commands
const (
	cmdPing = iota
	//SET
	cmdSetRXFreq
	cmdSetTXFreq
	cmdSetTXPower
	cmdSetReserved
	cmdSetFreqCorr
	cmdSetAFC
	cmdSetTXStart
	cmdSetRX
)

// const (
//
//	//GET
//	cmdGetIdent = iota + 0x80
//	cmdGetCaps
//	cmdGetRXFreq
//	cmdGetTXFreq
//	cmdGetTXPower
//	cmdGetFreqCorr
//
// )

const (
	trxIdle = iota
	trxRX
	trxTX
)

const txTimeout = 200 * time.Millisecond

type Line interface {
	SetValue(value int) error
	Close() error
}

type CC1200Modem struct {
	modem     io.ReadWriteCloser
	rxSymbols chan float32
	// txSymbols chan float32
	s2s SymbolToSample

	trxMutex  sync.Mutex
	trxState  int
	txTimer   *time.Timer
	cmdSource chan byte
	nRST      Line
	paEnable  Line
	boot0     Line
	debugLog  *os.File
}

func NewCC1200Modem(
	port string,
	nRSTPin int,
	paEnablePin int,
	boot0Pin int,
	baudRate int) (*CC1200Modem, error) {
	ret := CC1200Modem{
		rxSymbols: make(chan float32),
		s2s:       NewSymbolToSample(rrcTaps5, TXSymbolScalingCoeff*transmitGain, false, 5),
		cmdSource: make(chan byte),
	}
	ret.txTimer = time.AfterFunc(txTimeout, ret.stopTX)
	// Stop it until we transmit
	ret.txTimer.Stop()
	ret.trxState = trxIdle
	var err error
	fi, err := os.Stat(port)
	if err != nil {
		return nil, fmt.Errorf("modem stat: %w", err)
	}
	// err = ret.gpioSetup(nRSTPin, paEnablePin, boot0Pin)
	// if err != nil {
	// 	return nil, err
	// }
	if fi.Mode()&os.ModeSocket == os.ModeSocket {
		log.Printf("[DEBUG] Opening emulator")
		ret.modem, err = net.Dial("unix", port)
		if err != nil {
			return nil, fmt.Errorf("modem socket open: %w", err)
		}
		// This is the emulator so don't initialize GPIO
	} else {
		log.Printf("[DEBUG] Opening modem")
		err = ret.gpioSetup(nRSTPin, paEnablePin, boot0Pin)
		if err != nil {
			return nil, err
		}
		mode := &serial.Mode{
			BaudRate: baudRate,
		}
		ret.modem, err = serial.Open(port, mode)
		if err != nil {
			return nil, fmt.Errorf("modem open: %w", err)
		}
	}
	rxSource := make(chan int8, samplesPerSecond)
	ret.rxSymbols, err = ret.rxPipeline(rxSource)
	if err != nil {
		return nil, fmt.Errorf("rx pipeline setup: %w", err)
	}
	go ret.processReceivedData(rxSource)

	_, err = ret.commandWithResponse([]byte{cmdPing, 2})
	if err != nil {
		return nil, fmt.Errorf("test PING: %w", err)
	}
	// ret.debugLog, err = os.OpenFile("/home/jim/debug.sym", os.O_CREATE|os.O_WRONLY, 0644)
	// if err != nil {
	// 	log.Printf("[DEBUG] Failure opening debug log: %v", err)
	// } else {
	// 	log.Printf("[DEBUG] Opened debug log: %v", ret.debugLog)
	// }
	return &ret, nil
}

func (m *CC1200Modem) processReceivedData(rxSource chan int8) {
	buf := make([]byte, 1)
	for {
		// log.Printf("[DEBUG] processReceivedData Read()")
		n, err := m.modem.Read(buf)
		if n > 0 {
			// log.Printf("[DEBUG] processReceivedData read %x, trxState: %d", buf[0], m.trxState.Load())
			m.trxMutex.Lock()
			if m.trxState == trxRX {
				m.trxMutex.Unlock()
				select {
				case rxSource <- int8(buf[0]):
					// sent
					// log.Printf("[DEBUG] processReceivedData rxSource <- : %x", buf[0])
				default:
					// pipeline is full, so drop it
					log.Printf("[DEBUG] processReceivedData dropped rx: %x", buf[0])
				}
			} else {
				m.trxMutex.Unlock()
				// log.Printf("[DEBUG] processReceivedData cmdSource <- : %x", buf[0])
				m.cmdSource <- buf[0]
			}
		}
		if err != nil {
			log.Printf("[ERROR] Error reading from modem: %v", err)
			break
		}
	}
}
func (m *CC1200Modem) rxPipeline(sampleSource chan int8) (chan float32, error) {
	// modem samples -> DC filter --> RRC filter & scale
	var err error
	dcf, err := NewDCFilter(sampleSource, len(rrcTaps5))
	if err != nil {
		return nil, fmt.Errorf("dc filter: %w", err)
	}
	s2s := NewSampleToSymbol(dcf.Source(), rrcTaps5, RXSymbolScalingCoeff)
	// ds, err := NewDownsampler(s2s.Source(), 5, 0)
	// if err != nil {
	// 	return nil, fmt.Errorf("downsampler: %w", err)
	// }
	return s2s.Source(), nil
}

func (m *CC1200Modem) setNRSTGPIO(set bool) error {
	if m.nRST == nil {
		// Emulation mode
		return nil
	}
	log.Printf("[DEBUG] setNRSTGPIO(%v)", set)
	if set {
		return m.nRST.SetValue(1)
	}
	return m.nRST.SetValue(0)
}

func (m *CC1200Modem) setPAEnableGPIO(set bool) error {
	if m.paEnable == nil {
		// Emulation mode
		return nil
	}
	log.Printf("[DEBUG] setPAEnableGPIO(%v)", set)
	if set {
		return m.paEnable.SetValue(1)
	}
	return m.paEnable.SetValue(0)
}

func (m *CC1200Modem) setBoot0GPIO(set bool) error {
	if m.boot0 == nil {
		// Emulation mode
		return nil
	}
	log.Printf("[DEBUG] setBoot0GPIO(%v)", set)
	if set {
		return m.boot0.SetValue(1)
	}
	return m.boot0.SetValue(0)
}

// Reset the modem
func (m *CC1200Modem) Reset() error {
	log.Print("[DEBUG] modem Reset()")
	err1 := m.setBoot0GPIO(false)
	err2 := m.setPAEnableGPIO(false)
	err3 := m.setNRSTGPIO(false)
	time.Sleep(50 * time.Millisecond)
	err4 := m.setNRSTGPIO(true)
	errs := errors.Join(err1, err2, err3, err4)
	if errs != nil {
		return fmt.Errorf("modem reset: %w", errs)
	}
	return nil
}

// Close the modem
func (m *CC1200Modem) Close() error {
	log.Print("[DEBUG] modem Close()")
	m.stopRX()
	m.stopTX()
	m.nRST.Close()
	m.paEnable.Close()
	m.boot0.Close()
	if m.debugLog != nil {
		m.debugLog.Close()
	}
	return m.modem.Close()
}

// Read received symbols
func (m *CC1200Modem) Read(buf []byte) (n int, err error) {
	// log.Printf("[DEBUG] Modem.read requested %d bytes", len(buf))
	sBuf := make([]float32, len(buf)/4)
	for i := range sBuf {
		sBuf[i] = <-m.rxSymbols
	}
	sb, err := binary.Append(nil, binary.LittleEndian, sBuf)
	if err != nil {
		return 0, fmt.Errorf("append symbol: %w", err)
	}
	cnt := copy(buf, sb)
	// log.Printf("[DEBUG] Modem.read returned  %d bytes", cnt)
	return cnt, nil
}

// Send symbols to transmit. If no symbols are received for more than `txEndDuration` milliseconds,
// the transmission will end.
// func (m *CC1200Modem) Write(b []byte) (n int, err error) {
// 	symbols := make([]float32, len(b)/4)
// 	n, err = binary.Decode(b, binary.LittleEndian, symbols)
// 	if err != nil {
// 		err = fmt.Errorf("decode symbols: %w", err)
// 		return
// 	}
// 	// log.Printf("[DEBUG] Write symbols: % f", symbols)
// 	for _, s := range symbols {
// 		m.txSymbols <- s
// 	}
// 	// m.updateTXTimeout()
// 	if n < len(b) {
// 		// should only happen if len(b) is not a multiple of 4, i.e. the last symbol is incomplete
// 		err = fmt.Errorf("malformed transmit stream")
// 	}
// 	return
// }

func (m *CC1200Modem) TransmitPacket(p Packet) error {
	log.Printf("[DEBUG] TransmitPacket: %v", p)
	m.stopRX()
	time.Sleep(2 * time.Millisecond)
	m.startTX()
	time.Sleep(10 * time.Millisecond)

	var syms []Symbol
	//fill preamble
	syms = AppendPreamble(syms, lsfPreamble)
	err := m.writeSymbols(syms)
	if err != nil {
		return fmt.Errorf("failed to send preamble: %w", err)
	}
	syms, err = generateLSFSymbols(p.LSF)
	if err != nil {
		return fmt.Errorf("failed to generate LSF symbols: %w", err)
	}
	err = m.writeSymbols(syms)
	if err != nil {
		return fmt.Errorf("failed to send LSF: %w", err)
	}

	chunkCnt := 0
	packetData := p.PayloadBytes()
	for bytesLeft := len(packetData); bytesLeft > 0; bytesLeft -= 25 {
		syms = AppendSyncword(syms, PacketSync)
		chunk := make([]byte, 25+1) // 25 bytes from the packet plus 6 bits of metadata
		if bytesLeft > 25 {
			// not the last chunk
			copy(chunk, packetData[chunkCnt*25:chunkCnt*25+25])
			chunk[25] = byte(chunkCnt << 2)
		} else {
			// last chunk
			copy(chunk, packetData[chunkCnt*25:chunkCnt*25+bytesLeft])
			//EOT bit set to 1, set counter to the amount of bytes in this (the last) chunk
			if bytesLeft%25 == 0 {
				chunk[25] = (1 << 7) | ((25) << 2)
			} else {
				chunk[25] = uint8((1 << 7) | ((bytesLeft % 25) << 2))
			}
		}
		//encode the packet chunk
		b, err := ConvolutionalEncode(chunk, PacketPuncturePattern, PacketModeFinalBit)
		if err != nil {
			return fmt.Errorf("unable to encode packet: %w", err)
		}
		encodedBits := NewBits(b)
		rfBits := InterleaveBits(encodedBits)
		rfBits = RandomizeBits(rfBits)
		// Append chunk to the output
		syms = AppendBits(syms, rfBits)
		err = m.writeSymbols(syms)
		if err != nil {
			return fmt.Errorf("failed to send: %w", err)
		}
		time.Sleep(40 * time.Millisecond)
		chunkCnt++
	}
	syms = AppendEOT(syms)
	err = m.writeSymbols(syms)
	if err != nil {
		return fmt.Errorf("failed to send EOT: %w", err)
	}
	log.Printf("[DEBUG] Finished TransmitPacket")
	time.Sleep(10 * 40 * time.Millisecond)
	log.Printf("[DEBUG] Finished TransmitPacket wait")
	m.stopTX()
	m.Start()
	return nil
}

func (m *CC1200Modem) TransmitVoiceStream(sd StreamDatagram) error {
	m.trxMutex.Lock()
	if m.trxState != trxTX {
		// First frame
		m.trxMutex.Unlock()
		log.Printf("[DEBUG] Sending first frame of stream %x, fn %d, lsf: %v", sd.StreamID, sd.FrameNumber, sd.LSF)
		m.stopRX()
		time.Sleep(2 * time.Millisecond)
		m.startTX()
		time.Sleep(10 * time.Millisecond)

		var syms []Symbol
		//fill preamble
		syms = AppendPreamble(syms, lsfPreamble)
		err := m.writeSymbols(syms)
		if err != nil {
			return fmt.Errorf("failed to send preamble: %w", err)
		}
		syms, err = generateLSFSymbols(sd.LSF)
		if err != nil {
			return fmt.Errorf("failed to generate LSF symbols: %w", err)
		}
		err = m.writeSymbols(syms)
		if err != nil {
			return fmt.Errorf("failed to send LSF: %w", err)
		}
		syms, err = generateStreamSymbols(sd)
		if err != nil {
			return fmt.Errorf("failed to generate LSF symbols: %w", err)
		}
		err = m.writeSymbols(syms)
		if err != nil {
			return fmt.Errorf("failed to send stream frame: %w", err)
		}
	} else {
		m.trxMutex.Unlock()
		m.txTimer.Reset(txTimeout)
		log.Printf("[DEBUG] Sending frame of stream %x, fn %d", sd.StreamID, sd.FrameNumber)
		syms, err := generateStreamSymbols(sd)
		if err != nil {
			return fmt.Errorf("failed to generate LSF symbols: %w", err)
		}
		err = m.writeSymbols(syms)
		if err != nil {
			return fmt.Errorf("failed to send stream frame: %w", err)
		}
	}
	if sd.LastFrame {
		// send EOT
		log.Printf("[DEBUG] Sending EOT for stream %x, fn %d", sd.StreamID, sd.FrameNumber)
		syms := AppendEOT(nil)
		err := m.writeSymbols(syms)
		if err != nil {
			return fmt.Errorf("failed to send EOT: %w", err)
		}
		log.Printf("[DEBUG] Finished TransmitVoiceStream")
		time.Sleep(10 * 40 * time.Millisecond)
		log.Printf("[DEBUG] Finished TransmitVoiceStream wait")
		m.stopTX()
		m.Start()
	}
	return nil
}

func generateLSFSymbols(l LSF) ([]Symbol, error) {
	syms := AppendSyncword(nil, LSFSync)

	b, err := ConvolutionalEncode(l.ToBytes(), LSFPuncturePattern, LSFFinalBit)
	if err != nil {
		return nil, fmt.Errorf("unable to encode LSF: %w", err)
	}
	encodedBits := NewBits(b)
	// encodedBits[0:len(b)] = b[:]
	rfBits := InterleaveBits(encodedBits)
	rfBits = RandomizeBits(rfBits)
	// Append LSF to the output
	syms = AppendBits(syms, rfBits)
	return syms, nil
}

func generateStreamSymbols(sd StreamDatagram) ([]Symbol, error) {
	syms := AppendSyncword(nil, StreamSync)
	lich := extractLICH(int((sd.FrameNumber&0x7fff)%6), sd.LSF)
	encodedLICH := EncodeLICH(lich)
	lichBits := unpackBits(encodedLICH)
	b, err := ConvolutionalEncodeStream(lichBits, sd)
	if err != nil {
		return syms, fmt.Errorf("encode stream: %w", err)
	}
	encodedBits := NewBits(b)
	rfBits := InterleaveBits(encodedBits)
	rfBits = RandomizeBits(rfBits)
	syms = AppendBits(syms, rfBits)
	// log.Printf("[DEBUG] len(syms): %d, syms: [% v]", len(syms), syms)
	// d := NewDecoder()
	// frameData, li, fn, lichCnt, vd := d.decodeStreamFrame(syms[8:])
	// log.Printf("[DEBUG] frameData: [% 2x], lich: %x, lichCnt: %d, fn: %x, FN: %d, vd: %1.1f", frameData, li, lichCnt, fn, (fn>>8)|((fn&0xFF)<<8), vd)
	return syms, nil
}

func extractLICH(lichCnt int, lsf LSF) []byte {
	lich := lsf.ToBytes()[lichCnt*5 : lichCnt*5+5]
	return append(lich, byte(lichCnt)<<5)
}

func unpackBits(in []byte) []Bit {
	bits := make([]Bit, 8*len(in))
	for i := range in {
		for j := range 8 {
			bits[i*8+j].Set((in[i] >> (7 - j)) & 1)
		}
	}

	return bits
}
func (m *CC1200Modem) startTX() error {
	log.Printf("[DEBUG] startTX()")
	err := m.command([]byte{cmdSetTXStart, 2})
	if err != nil {
		return fmt.Errorf("start TX: %w", err)
	}
	err = m.setPAEnableGPIO(true)
	if err != nil {
		log.Printf("[DEBUG] Start TX PAEnable: %v", err)
	}
	m.trxMutex.Lock()
	m.trxState = trxTX
	m.trxMutex.Unlock()
	m.txTimer.Reset(txTimeout)
	return nil
}

func (m *CC1200Modem) stopTX() {
	log.Print("[DEBUG] modem stopTX()")
	m.trxMutex.Lock()
	// Only stop if we've started
	if m.trxState == trxTX {
		m.trxMutex.Unlock()
		log.Print("[DEBUG] modem stopping TX")
		err := m.setPAEnableGPIO(false)
		if err != nil {
			log.Printf("[DEBUG] End TX PAEnable: %v", err)
		}
		m.trxMutex.Lock()
		m.trxState = trxIdle
	}
	m.trxMutex.Unlock()
	m.txTimer.Stop()
}

func (m *CC1200Modem) SetTXFreq(freq uint32) error {
	log.Printf("[DEBUG] SetTXFreq(%v)", freq)
	var err error
	cmd := []byte{cmdSetTXFreq, 0}
	cmd, err = binary.Append(cmd, binary.LittleEndian, freq)
	if err != nil {
		return fmt.Errorf("encode set TX freq: %w", err)
	}
	err = m.commandWithErrResponse(cmd)
	if err != nil {
		return fmt.Errorf("send set TX freq: %w", err)
	}
	return nil
}
func (m *CC1200Modem) SetTXPower(dbm float32) error {
	log.Printf("[DEBUG] SetTXPower(%v)", dbm)
	var err error
	cmd := []byte{cmdSetTXPower, 0}
	cmd, err = binary.Append(cmd, binary.LittleEndian, int8(dbm*4))
	if err != nil {
		return fmt.Errorf("encode set TX power: %w", err)
	}
	err = m.commandWithErrResponse(cmd)
	if err != nil {
		return fmt.Errorf("send set TX power: %w", err)
	}
	return nil
}

func (m *CC1200Modem) Start() error {
	log.Printf("[DEBUG] Start()")
	// Sometimes we don't go into RX, so try stopping first
	// m.stopRX()
	m.trxMutex.Lock()
	m.trxState = trxRX
	m.trxMutex.Unlock()
	m.clearResponseBuf()
	var err error
	cmd := []byte{cmdSetRX, 0, 1}
	err = m.command(cmd)
	if err != nil {
		return fmt.Errorf("send set RX start error: %w", err)
	}
	return nil
}

func (m *CC1200Modem) stopRX() error {
	m.trxMutex.Lock()
	// Only stop if we've started
	if m.trxState == trxRX {
		m.trxMutex.Unlock()
		log.Printf("[DEBUG] stopRX()")
		var err error
		cmd := []byte{cmdSetRX, 0, 0}
		// Theoretically this returns a response, but how to find it in the received data
		err = m.command(cmd)
		if err != nil {
			return fmt.Errorf("send set RX stop: %w", err)
		}
		m.clearResponseBuf()
		m.trxMutex.Lock()
		m.trxState = trxIdle
	}
	m.trxMutex.Unlock()
	return nil
}
func (m *CC1200Modem) SetRXFreq(freq uint32) error {
	log.Printf("[DEBUG] SetRXFreq(%v)", freq)
	var err error
	cmd := []byte{cmdSetRXFreq, 0}
	cmd, err = binary.Append(cmd, binary.LittleEndian, freq)
	if err != nil {
		return fmt.Errorf("encode set RX freq: %w", err)
	}
	err = m.commandWithErrResponse(cmd)
	if err != nil {
		return fmt.Errorf("send set RX freq: %w", err)
	}
	return nil
}
func (m *CC1200Modem) SetAFC(afc bool) error {
	log.Printf("[DEBUG] SetAFC(%v)", afc)
	var err error
	var a byte
	if afc {
		a = 1
	}
	cmd := []byte{cmdSetAFC, 0, a}
	err = m.commandWithErrResponse(cmd)
	if err != nil {
		return fmt.Errorf("send set AFC: %w", err)
	}
	return nil
}
func (m *CC1200Modem) SetFreqCorrection(corr int16) error {
	log.Printf("[DEBUG] SetFreqCorrection(%v)", corr)
	var err error
	cmd := []byte{cmdSetFreqCorr, 0}
	cmd, err = binary.Append(cmd, binary.LittleEndian, corr)
	if err != nil {
		return fmt.Errorf("encode set freq corr: %w", err)
	}
	err = m.commandWithErrResponse(cmd)
	if err != nil {
		return fmt.Errorf("send set freq corr: %w", err)
	}
	return nil
}
func (m *CC1200Modem) writeSymbols(symbols []Symbol) error {
	buf := m.s2s.Transform(symbols)
	if m.debugLog != nil {
		_, err := m.debugLog.Write(buf)
		if err != nil {
			log.Printf("[DEBUG] Failed to write to debug log: %v", err)
		}
	}
	_, err := m.modem.Write(buf)
	return err
}
func (m *CC1200Modem) commandWithErrResponse(cmd []byte) error {
	var err error
	var respErr int
	respBuf, err := m.commandWithResponse(cmd)
	if err != nil {
		return fmt.Errorf("commandWithResponse error: %w", err)
	}
	// log.Printf("[DEBUG] respBuf: % x", respBuf)
	switch len(respBuf) {
	case 1:
		respErr = int(respBuf[0])
	case 4:
		_, err = binary.Decode(respBuf, binary.LittleEndian, respErr)
		if err != nil {
			return fmt.Errorf("parse modem response: %d", respErr)
		}
	default:
		return fmt.Errorf("unexpected response: %#v", respBuf)
	}
	// log.Printf("[DEBUG] respErr: %#v", respErr)
	if respErr != 0 {
		return fmt.Errorf("modem response: %d", respErr)
	}
	return nil
}

func (m *CC1200Modem) command(cmd []byte) error {
	if len(cmd) < 2 {
		return fmt.Errorf("command cmd length < 2")
	}
	cmd[1] = byte(len(cmd))
	var err error
	// log.Printf("[DEBUG] modem command(): % 2x", cmd)
	_, err = m.modem.Write(cmd)
	if err != nil {
		return fmt.Errorf("command: %w", err)
	}
	return nil
}
func (m *CC1200Modem) commandWithResponse(cmd []byte) ([]byte, error) {
	// log.Printf("[DEBUG] commandWithResponse() sending: % 2x", cmd)
	m.clearResponseBuf()
	err := m.command(cmd)
	if err != nil {
		return nil, err
	}
	resp, err := m.commandResponse()
	if err != nil {
		return nil, fmt.Errorf("commandWithResponse(): %w", err)
	}
	// log.Printf("[DEBUG] commandWithResponse() received: % 2x", resp)
	return resp, nil
}

func (m *CC1200Modem) clearResponseBuf() {
	for {
		select {
		case b := <-m.cmdSource:
			log.Printf("[DEBUG] CC1200 modem discarding response: %2x", b)
		default:
			return
		}
	}
}
func (m *CC1200Modem) commandResponse() ([]byte, error) {
	buf := make([]byte, 2)
	// log.Printf("[DEBUG] reading 2 bytes")
	buf[0] = <-m.cmdSource
	buf[1] = <-m.cmdSource
	// log.Printf("[DEBUG] reading rest: %d", buf[1]-2)
	buf = make([]byte, buf[1]-2)
	for i := range buf {
		buf[i] = <-m.cmdSource
	}
	// log.Printf("[DEBUG] commandResponse(): % x", buf)
	return buf, nil
}
