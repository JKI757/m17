package gateway

import (
	"github.com/jancona/m17/pkg/inet"
	"github.com/jancona/m17/pkg/m17"
)

// Dependencies contains the runtime services required by a gateway.
// The command package creates these values.
type Dependencies struct {
	Modem         m17.Modem
	Reflector     Reflector
	Hosts         map[string]m17.Host
	OverrideHosts map[string]m17.Host
	Logger        inet.Logger
}

// Reflector sends M17 traffic to the configured Internet reflector.
type Reflector interface {
	SendPacket(m17.Packet) error
	SendStream(m17.StreamDatagram) error
}

// Validate checks required runtime dependencies.
func (d Dependencies) Validate() error {
	if d.Modem == nil {
		return ErrMissingModem
	}
	return nil
}
