package gateway

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jancona/m17/pkg/m17"
)

// Service is the gateway application boundary.
// Its behavior will move here from the command package.
type Service struct {
	config       Config
	dependencies Dependencies

	ReflectorServer string
	ReflectorPort   uint

	stateMutex      sync.Mutex
	state           State
	lastLSF         *m17.LSF
	lastStreamID    uint16
	lastFrameTimer  *time.Timer
	echoStream      []m17.StreamDatagram
	encodedCallsign m17.EncodedCallsign
	inetClient      *m17.InetClient
	audioClips      map[string][]byte
}

// State describes the current gateway traffic direction.
type State int

const (
	Idle State = iota
	RFStreamRX
	RFPacketRX
	NetStreamRX
	NetPacketRX
	Echo
	LocalCommand
)

// New creates a gateway service from typed settings and dependencies.
func New(config Config, dependencies Dependencies) (*Service, error) {
	if err := errors.Join(config.Validate(), dependencies.Validate()); err != nil {
		return nil, err
	}
	host, ok := dependencies.OverrideHosts[config.ReflectorName]
	if !ok {
		host, ok = dependencies.Hosts[config.ReflectorName]
	}
	if !ok {
		return nil, fmt.Errorf("reflector %s not found", config.ReflectorName)
	}
	encodedCallsign, err := m17.EncodeCallsign(config.Callsign)
	if err != nil {
		return nil, err
	}
	service := &Service{
		config:          config,
		dependencies:    dependencies,
		ReflectorServer: host.Server,
		ReflectorPort:   host.Port,
		state:           Idle,
		lastStreamID:    0xffff,
		encodedCallsign: *encodedCallsign,
	}
	if config.AudioDir != "" {
		if err := service.LoadAudioClips(config.AudioDir); err != nil {
			return nil, err
		}
	}
	return service, nil
}

// State returns the current traffic state.
func (s *Service) State() State {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	return s.state
}

// SetState changes the current traffic state.
func (s *Service) SetState(state State) {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	s.state = state
}

// SetReflector attaches the Internet reflector after the service callbacks are available.
func (s *Service) SetReflector(reflector Reflector) {
	s.dependencies.Reflector = reflector
}

// Start begins radio frame decoding.
func (s *Service) Start() error {
	if s.inetClient == nil {
		client, err := m17.NewInetClient(
			s.config.ReflectorName,
			s.ReflectorServer,
			s.ReflectorPort,
			s.config.ReflectorModule,
			s.config.Callsign,
			s.dependencies.Logger,
			s.ReceivePacket,
			s.ReceiveStream,
		)
		if err != nil {
			return fmt.Errorf("create reflector client: %w", err)
		}
		s.inetClient = client
		s.SetReflector(client)
		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect reflector client: %w", err)
		}
	}
	decoder := m17.NewDecoder(
		s.ReceivedRFLSF,
		s.ReceivedRFStreamFrame,
		s.ReceivedRFStreamLICH,
		s.ReceivedRFStreamEOT,
		s.ReceivedRFPacket,
	)
	s.dependencies.Modem.StartDecoding(decoder.DecodeFrame)
	return nil
}

// Close stops the reflector client and modem.
func (s *Service) Close() {
	if s.inetClient != nil {
		s.inetClient.Close()
	}
	if s.dependencies.Modem != nil {
		s.dependencies.Modem.Close()
	}
}
