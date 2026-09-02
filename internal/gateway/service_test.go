package gateway

import (
	"testing"

	"github.com/jancona/m17/pkg/m17"
)

type testModem struct {
	packets []m17.Packet
	streams []m17.StreamDatagram
}

func (m *testModem) StartDecoding(func(uint16, []m17.SoftBit)) {}
func (m *testModem) Start() error                              { return nil }
func (m *testModem) Reset() error                              { return nil }
func (m *testModem) Close() error                              { return nil }
func (m *testModem) TransmitPacket(packet m17.Packet) error {
	m.packets = append(m.packets, packet)
	return nil
}
func (m *testModem) TransmitVoiceStream(stream m17.StreamDatagram) error {
	m.streams = append(m.streams, stream)
	return nil
}

type testReflector struct {
	packets []m17.Packet
	streams []m17.StreamDatagram
}

func (r *testReflector) SendPacket(packet m17.Packet) error {
	r.packets = append(r.packets, packet)
	return nil
}
func (r *testReflector) SendStream(stream m17.StreamDatagram) error {
	r.streams = append(r.streams, stream)
	return nil
}

func newTestService(t *testing.T) (*Service, *testModem, *testReflector) {
	t.Helper()
	modem := &testModem{}
	reflector := &testReflector{}
	service, err := New(Config{Callsign: "N0CALL", ReflectorName: "TEST", ReflectorModule: "A"}, Dependencies{
		Modem: modem,
		Hosts: map[string]m17.Host{"TEST": {Name: "TEST", Server: "127.0.0.1", Port: 17000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	service.SetReflector(reflector)
	return service, modem, reflector
}

func TestServiceForwardsRFStream(t *testing.T) {
	service, _, reflector := newTestService(t)
	lsf, err := m17.NewLSF("@ALL", "N0CALL", m17.LSFTypeStream, m17.LSFDataTypeVoice, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ReceivedRFLSF(lsf, 0); err != nil {
		t.Fatal(err)
	}
	if service.State() != RFStreamRX {
		t.Fatalf("state = %v, want RFStreamRX", service.State())
	}
	if err := service.ReceivedRFStreamFrame(lsf, make([]byte, 16), 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	if len(reflector.streams) != 1 {
		t.Fatalf("stream count = %d, want 1", len(reflector.streams))
	}
}
