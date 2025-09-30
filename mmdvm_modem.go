package m17

import (
	"errors"
	"fmt"
	"io"
	"log"
	"slices"
	"sync"
	"time"

	"go.bug.st/serial"
	"gopkg.in/ini.v1"
)

var (
	ErrMMDVMReadTimeout         = errors.New("read timeout")
	ErrUnsupportedModemProtocol = errors.New("unsupported MMDVM protocol version")
	ErrModemNAK                 = errors.New("modem returned NAK")
)

var mmdvmValidSpeeds = []int{1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200, 230400, 460800}

const (
	mmdvmFrameStart   = 0xE0
	mmdvmGetVersion   = 0x00
	mmdvmGetStatus    = 0x01
	mmdvmSetConfig    = 0x02
	mmdvmSetMode      = 0x03
	mmdvmSetFrequency = 0x04

	mmdvmM17LinkSetup = 0x45
	mmdvmM17Stream    = 0x46
	mmdvmM17Packet    = 0x47
	mmdvmM17Lost      = 0x48
	mmdvmM17EOT       = 0x49

	mmdvmSerialData = 0x80

	mmdvmTransparent = 0x90
	// mmdvmQSOInfo     = 0x91

	mmdvmDebug1    = 0xF1
	mmdvmDebug2    = 0xF2
	mmdvmDebug3    = 0xF3
	mmdvmDebug4    = 0xF4
	mmdvmDebug5    = 0xF5
	mmdvmDebugDump = 0xFA
	mmdvmACK       = 0x70
	mmdvmNAK       = 0x7F

	mmdvmReadBufferLen = 2000

	mmdvmCapabilities1DSTAR  = 0x01
	mmdvmCapabilities1DMR    = 0x02
	mmdvmCapabilities1YSF    = 0x04
	mmdvmCapabilities1P25    = 0x08
	mmdvmCapabilities1NXDN   = 0x10
	mmdvmCapabilities1M17    = 0x20
	mmdvmCapabilities1FM     = 0x40
	mmdvmCapabilities2POCSAG = 0x01
	mmdvmCapabilities2AX25   = 0x02

	mmdvmModeIdle = 0
	// mmdvmModeDSTAR  = 1
	// mmdvmModeDMR    = 2
	// mmdvmModeYSF    = 3
	// mmdvmModeP25    = 4
	// mmdvmModeNXDN   = 5
	// mmdvmModePOCSAG = 6
	mmdvmModeM17 = 7

	// mmdvmModeFM = 10

	// mmdvmModeCW      = 98
	// mmdvmModeLockout = 99
	// mmdvmModeError   = 100
	// mmdvmModeQuit    = 110

	// mmdvmTagHeader = 0x00
	// mmdvmTagData   = 0x01
	// mmdvmTagLost   = 0x02
	// mmdvmTagEOT    = 0x03
)
const (
	mmdvmSerialStateStart = iota
	mmdvmSerialStateLength1
	mmdvmSerialStateLength2
	mmdvmSerialStateType
	mmdvmSerialStateData
)
const (
	mmdvmHWTypeMMDVM = iota
	mmdvmHWTypeDVMEGA
	mmdvmHWTypeMMDVM_ZUMSPOT
	mmdvmHWTypeMMDVM_HS_HAT
	mmdvmHWTypeMMDVM_HS_DUAL_HAT
	mmdvmHWTypeNANO_HOTSPOT
	mmdvmHWTypeNANO_DV
	mmdvmHWTypeD2RG_MMDVM_HS
	mmdvmHWTypeMMDVM_HS
	mmdvmHWTypeOPENGD77_HS
	mmdvmHWTypeSKYBRIDGE
)

type MMDVMModem struct {
	frameSink func(typ uint16, softBits []SoftBit)

	duplex     bool
	m17Enabled bool
	m17TXLevel float32
	m17TXHang  byte

	mutex sync.Mutex
	// protected by mutex
	port io.ReadWriteCloser
	exit bool
	// end protected by mutex

	capabilities        [2]byte
	hwType              byte
	protocolVersion     byte
	txTimer             *time.Timer
	lastTXData          time.Time
	lastStatusCheck     time.Time
	lastInactivityCheck time.Time
}

func NewMMDVMModem(
	rxFrequency uint32,
	txFrequency uint32,
	power float32,
	frequencyCorr int16,
	afc bool,
	modemCfg *ini.Section,
	duplex bool) (*MMDVMModem, error) {
	var protocolErr, portErr error
	protocol := modemCfg.Key("Protocol").In("BAD", []string{"uart"})
	if protocol == "BAD" {
		protocolErr = fmt.Errorf("modem Protocol value is '%s', must be 'uart'", modemCfg.Key("Protocol").String())
	}
	port := modemCfg.Key("Port").String()
	if port == "" {
		portErr = errors.New("modem Port must have a value")
	}
	speed, speedErr := modemCfg.Key("Speed").Int()
	if speedErr == nil && !slices.Contains(mmdvmValidSpeeds, speed) {
		speedErr = fmt.Errorf("modem Speed value is %d, must be one of %d", speed, mmdvmValidSpeeds)
	}

	var err error
	err = errors.Join(
		protocolErr,
		portErr,
		speedErr,
	)
	if err != nil {
		return nil, err
	}

	m := &MMDVMModem{
		duplex:     duplex,
		m17Enabled: true, // TODO: from config
		m17TXLevel: 50,
		m17TXHang:  5,
	}
	m.txTimer = time.AfterFunc(txTimeout, func() {
		log.Printf("[DEBUG] TX timeout")
		// ret.stopTX()
		// ret.Start()
	})
	m.lastTXData = time.Now()
	// Stop it until we transmit
	m.txTimer.Stop()
	log.Printf("[DEBUG] Opening modem")
	mode := &serial.Mode{
		BaudRate: speed,
	}
	p, err := serial.Open(port, mode)
	if err != nil {
		return nil, fmt.Errorf("modem open: %w", err)
	}
	p.SetReadTimeout(0) // Non-blocking reads
	m.port = p
	// rxSource := make(chan int8, samplesPerSecond)
	// ret.rxSymbols, err = ret.rxPipeline(rxSource)
	// if err != nil {
	// 	return nil, fmt.Errorf("rx pipeline setup: %w", err)
	// }
	// go ret.processReceivedData(rxSource)
	err = m.readVersion()
	if err != nil {
		return nil, err
	}
	err = m.setFrequency(rxFrequency, txFrequency, power)
	if err != nil {
		return nil, err
	}
	err = m.writeConfig()
	if err != nil {
		return nil, err
	}
	err = m.setMode(mmdvmModeM17)
	if err != nil {
		return nil, err
	}
	go m.run()
	return m, nil
}

func (m *MMDVMModem) isExit() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.exit
}
func (m *MMDVMModem) setExit(exit bool) {
	m.mutex.Lock()
	m.exit = exit
	m.mutex.Unlock()
}
func (m *MMDVMModem) run() {
	log.Printf("[DEBUG] modem start running")
	m.setExit(false)
	for !m.isExit() {
		if expired(&m.lastStatusCheck, 250*time.Millisecond) {
			// m.readStatus()
		}
		if expired(&m.lastInactivityCheck, 2*time.Second) {
			// close and reopen
		}
		m.mutex.Lock()
		responseType, buf, err := m.getResponse()
		m.mutex.Unlock()
		if err == ErrMMDVMReadTimeout {
			// nothing to do
		} else if err != nil {
			log.Printf("[DEBUG] Modem run getReponse() err: %v", err)
		} else {
			switch responseType {
			case mmdvmM17LinkSetup:
				log.Printf("[DEBUG] Received M17 LSF: [% 02x]", buf)
				sb := bytesToSoftBits(buf[3 : 46+3])
				// log.Printf("[DEBUG] M17 LSF Softbits: [% v]", sb)
				m.frameSink(LSFSync, sb)
			case mmdvmM17Stream:
				log.Printf("[DEBUG] Received M17 Stream frame: [% 02x]", buf)
				sb := bytesToSoftBits(buf[3 : 46+3])
				m.frameSink(StreamSync, sb)
			case mmdvmM17Packet:
				log.Printf("[DEBUG] Received M17 Packet frame: [% 02x]", buf)
				sb := bytesToSoftBits(buf[3 : 46+3])
				m.frameSink(PacketSync, sb)
			case mmdvmM17EOT:
				log.Printf("[DEBUG] Received M17 EOT: [% 02x]", buf)
				sb := bytesToSoftBits(buf[3 : 46+3])
				m.frameSink(EOTMarker, sb)
			case mmdvmM17Lost:
				log.Printf("[DEBUG] Received M17 Lost: [% 02x]", buf)
			case mmdvmGetStatus:
			case mmdvmTransparent:
			case mmdvmGetVersion:
			case mmdvmSerialData:
			case mmdvmACK:
			case mmdvmNAK:
				log.Printf("[WARN] Modem run getReponse() received a NAK, command: %02x, reason: %d", buf[0], buf[1])
			case mmdvmDebug1, mmdvmDebug2, mmdvmDebug3, mmdvmDebug4, mmdvmDebug5, mmdvmDebugDump:
			// if (m_type == MMDVM_DEBUG1) {
			// 	LogMessage("Debug: %.*s", m_length - m_offset - 0U, m_buffer + m_offset);
			// } else if (m_type == MMDVM_DEBUG2) {
			// 	short val1 = (m_buffer[m_length - 2U] << 8) | m_buffer[m_length - 1U];
			// 	LogMessage("Debug: %.*s %d", m_length - m_offset - 2U, m_buffer + m_offset, val1);
			// } else if (m_type == MMDVM_DEBUG3) {
			// 	short val1 = (m_buffer[m_length - 4U] << 8) | m_buffer[m_length - 3U];
			// 	short val2 = (m_buffer[m_length - 2U] << 8) | m_buffer[m_length - 1U];
			// 	LogMessage("Debug: %.*s %d %d", m_length - m_offset - 4U, m_buffer + m_offset, val1, val2);
			// } else if (m_type == MMDVM_DEBUG4) {
			// 	short val1 = (m_buffer[m_length - 6U] << 8) | m_buffer[m_length - 5U];
			// 	short val2 = (m_buffer[m_length - 4U] << 8) | m_buffer[m_length - 3U];
			// 	short val3 = (m_buffer[m_length - 2U] << 8) | m_buffer[m_length - 1U];
			// 	LogMessage("Debug: %.*s %d %d %d", m_length - m_offset - 6U, m_buffer + m_offset, val1, val2, val3);
			// } else if (m_type == MMDVM_DEBUG5) {
			// 	short val1 = (m_buffer[m_length - 8U] << 8) | m_buffer[m_length - 7U];
			// 	short val2 = (m_buffer[m_length - 6U] << 8) | m_buffer[m_length - 5U];
			// 	short val3 = (m_buffer[m_length - 4U] << 8) | m_buffer[m_length - 3U];
			// 	short val4 = (m_buffer[m_length - 2U] << 8) | m_buffer[m_length - 1U];
			// 	LogMessage("Debug: %.*s %d %d %d %d", m_length - m_offset - 8U, m_buffer + m_offset, val1, val2, val3, val4);
			// } else if (m_type == MMDVM_DEBUG_DUMP) {
			// 	CUtils::dump(1U, "Debug: Data", m_buffer + m_offset, m_length - m_offset);
			default:
				log.Printf("[DEBUG] Unexpected modem response, type: %2x, buf: [% 2x]", responseType, buf)
			}
		}

	}
	log.Printf("[DEBUG] modem stop running")
}

func bytesToSoftBits(buf []byte) []SoftBit {
	var ret []SoftBit
	for _, b := range buf {
		for range 8 {
			if b&0x80 == 0x80 {
				ret = append(ret, softTrue)
			} else {
				ret = append(ret, softFalse)
			}
			b <<= 1
		}
	}
	return ret
}
func expired(t *time.Time, duration time.Duration) bool {
	ret := time.Since(*t) > duration
	if ret {
		*t = time.Now()
	}
	return ret
}

func (m *MMDVMModem) readVersion() error {
	time.Sleep(2 * time.Second)
	m.mutex.Lock()
	defer m.mutex.Unlock()
	cmd := []byte{mmdvmFrameStart, 3, mmdvmGetVersion}
retry:
	for range 6 {
		time.Sleep(10 * time.Millisecond)
		log.Printf("[DEBUG] Trying GetVersion")
		_, err := m.port.Write(cmd)
		if err != nil {
			return fmt.Errorf("error writing GetVersion cmd: %w", err)
		}
		responseType, buf, err := m.getResponse()
		// log.Printf("[DEBUG] GetVersion response: [% x], err: %v", buf, err)
		if err == nil && responseType == mmdvmGetVersion {
			switch {
			case string(buf[1:1+6]) == "MMDVM " || string(buf[23:23+6]) == "MMDVM ":
				m.hwType = mmdvmHWTypeMMDVM
			case string(buf[1:1:6]) == "DVMEGA":
				m.hwType = mmdvmHWTypeDVMEGA
			case string(buf[1:1+7]) == "ZUMspot":
				m.hwType = mmdvmHWTypeMMDVM_ZUMSPOT
			case string(buf[1:1+12]) == "MMDVM_HS_Hat":
				m.hwType = mmdvmHWTypeMMDVM_HS_HAT
			case string(buf[1:1+17]) == "MMDVM_HS_Dual_Hat":
				m.hwType = mmdvmHWTypeMMDVM_HS_DUAL_HAT
			case string(buf[1:1+12]) == "Nano_hotSPOT":
				m.hwType = mmdvmHWTypeNANO_HOTSPOT
			case string(buf[1:1+7]) == "Nano_DV":
				m.hwType = mmdvmHWTypeNANO_DV
			case string(buf[1:1+13]) == "D2RG_MMDVM_HS":
				m.hwType = mmdvmHWTypeD2RG_MMDVM_HS
			case string(buf[1:1+9]) == "MMDVM_HS-":
				m.hwType = mmdvmHWTypeMMDVM_HS
			case string(buf[1:1+11]) == "OpenGD77_HS":
				m.hwType = mmdvmHWTypeOPENGD77_HS
			case string(buf[1:1+9]) == "SkyBridge":
				m.hwType = mmdvmHWTypeSKYBRIDGE
			}
			m.protocolVersion = buf[0]
			switch m.protocolVersion {
			case 1:
				log.Printf("[INFO] MMDVM protocol version: 1, description: %s", buf[1:])
				m.capabilities[0] =
					mmdvmCapabilities1DSTAR | mmdvmCapabilities1DMR | mmdvmCapabilities1YSF |
						mmdvmCapabilities1P25 | mmdvmCapabilities1NXDN | mmdvmCapabilities1M17
				m.capabilities[1] = mmdvmCapabilities2POCSAG
				break retry
			case 2:
				log.Printf("[INFO] MMDVM protocol version: 2, description: %s", buf[20:])
				switch buf[6] {
				case 0:
					log.Printf("[INFO] CPU: Atmel ARM, UDID: %02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X", buf[4], buf[5], buf[6], buf[7], buf[8], buf[9], buf[10], buf[11], buf[12], buf[13], buf[14], buf[15], buf[16], buf[17], buf[18], buf[19])
				case 1:
					log.Printf("[INFO] CPU: NXP ARM, UDID: %02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X", buf[4], buf[5], buf[6], buf[7], buf[8], buf[9], buf[10], buf[11], buf[12], buf[13], buf[14], buf[15], buf[16], buf[17], buf[18], buf[19])
				case 2:
					log.Printf("[INFO] CPU: ST-Micro ARM, UDID: %02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X", buf[4], buf[5], buf[6], buf[7], buf[8], buf[9], buf[10], buf[11], buf[12], buf[13], buf[14], buf[15])
				default:
					log.Printf("[INFO] CPU: Unknown type: %d", buf[3])
				}
				m.capabilities[0] = buf[1]
				m.capabilities[1] = buf[2]
				break retry
			default:
				log.Printf("[ERROR] Unsupported MMDVM protocol version: %d", m.protocolVersion)
				return ErrUnsupportedModemProtocol
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
	modeText := "Modes:"
	if (m.capabilities[0] & mmdvmCapabilities1DSTAR) == mmdvmCapabilities1DSTAR {
		modeText += " D-Star"
	}
	if (m.capabilities[0] & mmdvmCapabilities1DMR) == mmdvmCapabilities1DMR {
		modeText += " DMR"
	}
	if (m.capabilities[0] & mmdvmCapabilities1YSF) == mmdvmCapabilities1YSF {
		modeText += " YSF"
	}
	if (m.capabilities[0] & mmdvmCapabilities1P25) == mmdvmCapabilities1P25 {
		modeText += " P25"
	}
	if (m.capabilities[0] & mmdvmCapabilities1NXDN) == mmdvmCapabilities1NXDN {
		modeText += " NXDN"
	}
	if (m.capabilities[0] & mmdvmCapabilities1M17) == mmdvmCapabilities1M17 {
		modeText += " M17"
	}
	if (m.capabilities[0] & mmdvmCapabilities1FM) == mmdvmCapabilities1FM {
		modeText += " FM"
	}
	if (m.capabilities[0] & mmdvmCapabilities2POCSAG) == mmdvmCapabilities2POCSAG {
		modeText += " POCSAG"
	}
	if (m.capabilities[0] & mmdvmCapabilities2AX25) == mmdvmCapabilities2AX25 {
		modeText += " AX.25"
	}
	log.Printf("[INFO] %s", modeText)
	return nil
}

func (m *MMDVMModem) setFrequency(rxFreq, txFreq uint32, power float32) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	log.Printf("[DEBUG] setFrequency(%d, %d, %f)", rxFreq, txFreq, power)

	cmd := make([]byte, 12)
	cmd[0] = mmdvmFrameStart
	cmd[1] = byte(len(cmd))
	cmd[2] = mmdvmSetFrequency
	cmd[3] = 0x00

	cmd[4] = byte((rxFreq >> 0) & 0xFF)
	cmd[5] = byte((rxFreq >> 8) & 0xFF)
	cmd[6] = byte((rxFreq >> 16) & 0xFF)
	cmd[7] = byte((rxFreq >> 24) & 0xFF)

	cmd[8] = byte((txFreq >> 0) & 0xFF)
	cmd[9] = byte((txFreq >> 8) & 0xFF)
	cmd[10] = byte((txFreq >> 16) & 0xFF)
	cmd[11] = byte((txFreq >> 24) & 0xFF)

	log.Printf("[DEBUG] cmd: [% x]", cmd)
	_, err := m.port.Write(cmd)
	if err != nil {
		return fmt.Errorf("error writing setFrequency cmd: %w", err)
	}

	err = m.getEmptyResponse()
	return err
}

func (m *MMDVMModem) writeConfig() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	switch m.protocolVersion {
	case 1:
		return m.setProtocol1Config()
	// case 2:
	// 	return m.setProtocol2Config()
	default:
		return ErrUnsupportedModemProtocol
	}
}

func (m *MMDVMModem) setProtocol1Config() error {
	log.Printf("[DEBUG] setProtocol1Config()")
	cmd := make([]byte, 30)

	cmd[0] = mmdvmFrameStart

	cmd[1] = 26

	cmd[2] = mmdvmSetConfig

	cmd[3] = 0x00
	// if (m.rxInvert) {
	// 	cmd[3] |= 0x01
	// }
	// if (m.txInvert) {
	// 	cmd[3] |= 0x02
	// }
	// if (m.pttInvert) {
	// 	cmd[3] |= 0x04
	// }
	// if (m.ysfLoDev) {
	// 	cmd[3] |= 0x08
	// }
	// if (m.debug) {
	// 	cmd[3] |= 0x10
	// }
	// if (m.useCOSAsLockout) {
	// 	cmd[3] |= 0x20
	// }
	if !m.duplex {
		cmd[3] |= 0x80
	}
	cmd[4] = 0x00
	// if (m.dstarEnabled)
	// 	cmd[4] |= 0x01
	// if (m.dmrEnabled)
	// 	cmd[4] |= 0x02
	// if (m.ysfEnabled)
	// 	cmd[4] |= 0x04
	// if (m.p25Enabled)
	// 	cmd[4] |= 0x08
	// if (m.nxdnEnabled)
	// 	cmd[4] |= 0x10
	// if (m.pocsagEnabled)
	// 	cmd[4] |= 0x20
	if m.m17Enabled {
		cmd[4] |= 0x40
	}

	// cmd[5] = m.txDelay / 10 // In 10ms units

	cmd[6] = mmdvmModeIdle

	// cmd[7] = byte(m.rxLevel*2.55 + 0.5)

	// cmd[8] = byte(m.cwIdTXLevel*2.55 + 0.5)

	// cmd[9] = m.dmrColorCode

	// cmd[10] = m.dmrDelay

	cmd[11] = 128 // Was OscOffset

	// cmd[12] = byte(m.dstarTXLevel*2.55 + 0.5)
	// cmd[13] = byte(m.dmrTXLevel*2.55 + 0.5)
	// cmd[14] = byte(m.ysfTXLevel*2.55 + 0.5)
	// cmd[15] = byte(m.p25TXLevel*2.55 + 0.5)

	// cmd[16] = byte(m.txDCOffset + 128)
	// cmd[17] = byte(m.rxDCOffset + 128)

	// cmd[18] = byte(m.nxdnTXLevel*2.55 + 0.5)

	// cmd[19] = byte(m.ysfTXHang)

	// cmd[20] = byte(m.pocsagTXLevel*2.55 + 0.5)

	// cmd[21] = byte(m.fmTXLevel*2.55 + 0.5)

	// cmd[22] = byte(m.p25TXHang)

	// cmd[23] = byte(m.nxdnTXHang)

	cmd[24] = byte(m.m17TXLevel*2.55 + 0.5)

	cmd[25] = byte(m.m17TXHang)

	log.Printf("[DEBUG] cmd: [% x]", cmd)
	_, err := m.port.Write(cmd)
	if err != nil {
		return fmt.Errorf("error writing SetConfig cmd: %w", err)
	}

	err = m.getEmptyResponse()
	// m.playoutTimer.start();
	return err
}

// bool CModem::setConfig2()
// {
// 	assert(m.port != nullptr);

// 	unsigned char buffer[50];

// 	buffer[0] = MMDVM_FRAME_START;

// 	buffer[1] = 40

// 	buffer[2] = MMDVM_SET_CONFIG;

// 	buffer[3] = 0x00
// 	if (m.rxInvert)
// 		buffer[3] |= 0x01
// 	if (m.txInvert)
// 		buffer[3] |= 0x02
// 	if (m.pttInvert)
// 		buffer[3] |= 0x04
// 	if (m.ysfLoDev)
// 		buffer[3] |= 0x08
// 	if (m.debug)
// 		buffer[3] |= 0x10
// 	if (m.useCOSAsLockout)
// 		buffer[3] |= 0x20
// 	if (!m.duplex)
// 		buffer[3] |= 0x80

// 	buffer[4] = 0x00
// 	if (m.dstarEnabled)
// 		buffer[4] |= 0x01
// 	if (m.dmrEnabled)
// 		buffer[4] |= 0x02
// 	if (m.ysfEnabled)
// 		buffer[4] |= 0x04
// 	if (m.p25Enabled)
// 		buffer[4] |= 0x08
// 	if (m.nxdnEnabled)
// 		buffer[4] |= 0x10
// 	if (m.fmEnabled)
// 		buffer[4] |= 0x20
// 	if (m.m17Enabled)
// 		buffer[4] |= 0x40

// 	buffer[5] = 0x00
// 	if (m.pocsagEnabled)
// 		buffer[5] |= 0x01
// 	if (m.ax25Enabled)
// 		buffer[5] |= 0x02

// 	buffer[6] = m.txDelay / 10		// In 10ms units

// 	buffer[7] = MODE_IDLE;

// 	buffer[8] = byte(m.txDCOffset + 128);
// 	buffer[9] = byte(m.rxDCOffset + 128);

// 	buffer[10] = byte(m.rxLevel * 2.55F + 0.5F);

// 	buffer[11] = byte(m.cwIdTXLevel * 2.55F + 0.5F);
// 	buffer[12] = byte(m.dstarTXLevel * 2.55F + 0.5F);
// 	buffer[13] = byte(m.dmrTXLevel * 2.55F + 0.5F);
// 	buffer[14] = byte(m.ysfTXLevel * 2.55F + 0.5F);
// 	buffer[15] = byte(m.p25TXLevel * 2.55F + 0.5F);
// 	buffer[16] = byte(m.nxdnTXLevel * 2.55F + 0.5F);
// 	buffer[17] = byte(m.m17TXLevel * 2.55F + 0.5F);
// 	buffer[18] = byte(m.pocsagTXLevel * 2.55F + 0.5F);
// 	buffer[19] = byte(m.fmTXLevel * 2.55F + 0.5F);
// 	buffer[20] = byte(m.ax25TXLevel * 2.55F + 0.5F);
// 	buffer[21] = 0x00
// 	buffer[22] = 0x00

// 	buffer[23] = bytem.ysfTXHang;
// 	buffer[24] = bytem.p25TXHang;
// 	buffer[25] = bytem.nxdnTXHang;
// 	buffer[26] = bytem.m17TXHang;
// 	buffer[27] = 0x00
// 	buffer[28] = 0x00

// 	buffer[29] = m.dmrColorCode;
// 	buffer[30] = m.dmrDelay;

// 	buffer[31] = byte(m.ax25RXTwist + 128);
// 	buffer[32] = m.ax25TXDelay / 10		// In 10ms units
// 	buffer[33] = m.ax25SlotTime / 10		// In 10ms units
// 	buffer[34] = m.ax25PPersist;

// 	buffer[35] = 0x00
// 	buffer[36] = 0x00
// 	buffer[37] = 0x00
// 	buffer[38] = 0x00
// 	buffer[39] = 0x00

// 	// CUtils::dump(1U, "Written", buffer, 40U);

// 	int ret = m.port->write(buffer, 40U);
// 	if (ret != 40)
// 		return false;

// 	unsigned int count = 0
// 	RESP_TYPE_MMDVM resp;
// 	do {
// 		CThread::sleep(10U);

// 		resp = getResponse();
// 		if ((resp == RESP_TYPE_MMDVM::OK) && (m.buffer[2] != MMDVM_ACK) && (m.buffer[2] != MMDVM_NAK)) {
// 			count++;
// 			if (count >= MAX_RESPONSES) {
// 				LogError("The MMDVM is not responding to the SET_CONFIG command");
// 				return false;
// 			}
// 		}
// 	} while ((resp == RESP_TYPE_MMDVM::OK) && (m.buffer[2] != MMDVM_ACK) && (m.buffer[2] != MMDVM_NAK));

// 	// CUtils::dump(1U, "Response", m.buffer, m.length);

// 	if ((resp == RESP_TYPE_MMDVM::OK) && (m.buffer[2] == MMDVM_NAK)) {
// 		LogError("Received a NAK to the SET_CONFIG command from the modem");
// 		return false;
// 	}

// 	m.playoutTimer.start();

// 	return true;
// }

func (m *MMDVMModem) setMode(mode byte) error {
	log.Printf("[DEBUG] setMode(%02x)", mode)
	cmd := []byte{mmdvmFrameStart, 4, mmdvmSetMode, mode}
	_, err := m.port.Write(cmd)
	return err
}

func (m *MMDVMModem) getEmptyResponse() error {
	var responseType byte
	var err error
	for i := range 30 {
		time.Sleep(10 * time.Millisecond)
		responseType, _, err = m.getResponse()
		if i == 29 && err == nil && responseType != mmdvmACK && responseType != mmdvmNAK {
			log.Printf("[ERROR] Modem not responding to command")
			return errors.New("modem not responding")
		} else if (err != nil && err != ErrMMDVMReadTimeout) || responseType == mmdvmACK || responseType == mmdvmNAK {
			break
		}
	}
	if err == nil && responseType == mmdvmNAK {
		log.Print("[ERROR] Received a NAK to the SetConfig command from the modem")
		return ErrModemNAK
	}
	return err
}
func (m *MMDVMModem) getResponse() (byte, []byte, error) {
	var n int
	var err error
	var state byte
	var buffer []byte
	var offset int
	var length int
	var responseType byte

	state = mmdvmSerialStateStart
	buffer = make([]byte, mmdvmReadBufferLen)

	if state == mmdvmSerialStateStart {
		offset = 0
		n, err = m.port.Read(buffer[offset : offset+1])
		if err != nil {
			log.Printf("[ERROR] mmdvmSerialStateStart read error: %v", err)
			return responseType, nil, fmt.Errorf("mmdvmSerialStateStart read: %w", err)
		} else if n == 0 {
			return responseType, nil, ErrMMDVMReadTimeout
		}
		if buffer[0] != mmdvmFrameStart {
			return responseType, nil, ErrMMDVMReadTimeout
		}
		// log.Printf("[DEBUG] Modem mmdvmSerialStateStart read 0x%x", buffer[offset])
		state = mmdvmSerialStateLength1
		offset++
		length = 1
	}

	if state == mmdvmSerialStateLength1 {
		n, err = m.port.Read(buffer[offset : offset+1])
		if err != nil {
			log.Printf("[ERROR] mmdvmSerialStateLength1 read error: %v", err)
			state = mmdvmSerialStateStart
			return responseType, nil, fmt.Errorf("mmdvmSerialStateLength1 read: %w", err)
		} else if n == 0 {
			log.Printf("[DEBUG] mmdvmSerialStateLength1 read timeout")
			return responseType, nil, ErrMMDVMReadTimeout
		}
		// log.Printf("[DEBUG] Modem mmdvmSerialStateLength1 read %d", buffer[offset])
		length = int(buffer[offset])
		offset++
		if length == 0 {
			state = mmdvmSerialStateLength2
		} else {
			state = mmdvmSerialStateType
		}
	}

	if state == mmdvmSerialStateLength2 {
		n, err = m.port.Read(buffer[offset : offset+1])
		if err != nil {
			log.Printf("[ERROR] mmdvmSerialStateLength2 read error: %v", err)
			state = mmdvmSerialStateStart
			return responseType, nil, fmt.Errorf("mmdvmSerialStateLength2 read: %w", err)
		} else if n == 0 {
			log.Printf("[DEBUG] mmdvmSerialStateLength2 read timeout")
			return responseType, nil, ErrMMDVMReadTimeout
		}
		length = int(buffer[offset]) + 255
		// log.Printf("[DEBUG] Modem mmdvmSerialStateLength2 read %d, length %d", buffer[offset], length)
		offset++
		state = mmdvmSerialStateType
	}

	if state == mmdvmSerialStateType {
		n, err = m.port.Read(buffer[offset : offset+1])
		if err != nil {
			log.Printf("[ERROR] mmdvmSerialStateType read error: %v", err)
			state = mmdvmSerialStateStart
			return responseType, nil, fmt.Errorf("mmdvmSerialStateType read: %w", err)
		} else if n == 0 {
			log.Printf("[DEBUG] mmdvmSerialStateType read timeout")
			return responseType, nil, ErrMMDVMReadTimeout
		}
		responseType = buffer[offset]
		log.Printf("[DEBUG] Modem mmdvmSerialStateType read 0x%x", buffer[offset])
		offset++
		state = mmdvmSerialStateData
	}

	if state == mmdvmSerialStateData {
		time.Sleep(10 * time.Millisecond)
		for start := offset; start < length; start += n {
			n, err = m.port.Read(buffer[start:length])
			if err != nil {
				log.Printf("[ERROR] mmdvmSerialStateData read error: %v", err)
				state = mmdvmSerialStateStart
				return responseType, nil, fmt.Errorf("mmdvmSerialStateData read: %w", err)
			} else if n == 0 {
				log.Printf("[DEBUG] mmdvmSerialStateData read timeout")
				return responseType, nil, ErrMMDVMReadTimeout
			}
		}
	}
	buffer = buffer[offset:length]
	// log.Printf("[DEBUG] getResponse() responseType: 0x%x, buffer: [% x], len: %d, err: %v", responseType, buffer, len(buffer), err)
	return responseType, buffer, err
}

func (m *MMDVMModem) StartDecoding(sink func(typ uint16, softBits []SoftBit)) {
	m.frameSink = sink
}

// Reset the modem
func (m *MMDVMModem) Reset() error {
	log.Print("[DEBUG] modem Reset()")
	return nil
}

// Close the modem
func (m *MMDVMModem) Close() error {
	log.Print("[DEBUG] modem Close()")
	m.setExit(true)
	return m.port.Close()
}

func (m *MMDVMModem) TransmitPacket(p Packet) error {
	log.Printf("[DEBUG] TransmitPacket: %v", p)
	var bits []Bit
	var err error

	log.Printf("[DEBUG] TransmitPacket send LSF")
	err = m.transmitLSF(*p.LSF)
	if err != nil {
		return fmt.Errorf("failed to send packet LSF: %w", err)
	}

	chunkCnt := 0
	packetData := p.PayloadBytes()
	for bytesLeft := len(packetData); bytesLeft > 0; bytesLeft -= 25 {
		bits = unpackBits(PacketSyncBytes)
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
		encodedBits := NewPayloadBits(b)
		rfBits := InterleaveBits(encodedBits)
		rfBits = RandomizeBits(rfBits)
		// Append chunk to the output
		bits = append(bits, rfBits[:]...)
		err = m.writeBits(mmdvmM17Packet, bits)
		if err != nil {
			return fmt.Errorf("failed to send packet frame: %w", err)
		}
		chunkCnt++
	}
	err = m.writeEOT()
	if err != nil {
		return fmt.Errorf("failed to send EOT: %w", err)
	}
	log.Printf("[DEBUG] Finished TransmitPacket")
	return nil
}

func (m *MMDVMModem) transmitLSF(lsf LSF) error {
	log.Printf("[DEBUG] transmitLSF: %s", lsf)
	bits, err := generateLSFBits(lsf)
	if err != nil {
		return fmt.Errorf("failed to generate LSF bits: %w", err)
	}
	err = m.writeBits(mmdvmM17LinkSetup, bits)
	if err != nil {
		return fmt.Errorf("failed to send LSF: %w", err)
	}
	return nil
}

func (m *MMDVMModem) TransmitVoiceStream(sd StreamDatagram) error {
	log.Printf("[DEBUG] TransmitVoiceStream id: %04x, fn: %04x, last: %v", sd.StreamID, sd.FrameNumber, sd.LastFrame)
	var bits []Bit
	var err error

	if sd.FrameNumber == 0 && sd.LSF != nil { // first frame
		log.Printf("[DEBUG] TransmitVoiceStream send LSF")
		err = m.transmitLSF(*sd.LSF)
		if err != nil {
			return fmt.Errorf("failed to send stream LSF: %w", err)
		}
	}
	bits, err = generateStreamBits(sd)
	if err != nil {
		return fmt.Errorf("failed to generate stream bits: %w", err)
	}
	log.Printf("[DEBUG] TransmitVoiceStream sending bits")
	err = m.writeBits(mmdvmM17Stream, bits)
	if err != nil {
		return fmt.Errorf("failed to send stream frame: %w", err)
	}
	if sd.LastFrame {
		// send EOT
		log.Printf("[DEBUG] Sending EOT for stream %04x, fn %04x", sd.StreamID, sd.FrameNumber)
		err = m.writeEOT()
		if err != nil {
			return fmt.Errorf("failed to send EOT: %w", err)
		}
		log.Printf("[DEBUG] Finished TransmitVoiceStream")
	}
	return nil
}

func (m *MMDVMModem) writeBits(typ byte, bits []Bit) error {
	buf := packBits(bits)
	log.Printf("[DEBUG] writeBits type: %02x, len: %d, buf: % 02x", typ, len(buf), buf)
	cmd := []byte{mmdvmFrameStart, byte(4 + len(buf)), typ, 0}
	cmd = append(cmd, buf...)
	_, err := m.port.Write(cmd)
	return err
}
func (m *MMDVMModem) writeEOT() error {
	log.Printf("[DEBUG] writeEOT")
	cmd := []byte{mmdvmFrameStart, 3, mmdvmM17EOT}
	_, err := m.port.Write(cmd)
	return err
}

func (m *MMDVMModem) Start() error {
	log.Printf("[DEBUG] Start()")
	return nil
}
