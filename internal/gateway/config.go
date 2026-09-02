// Package gateway contains gateway application settings and behavior.
package gateway

// Config contains the settings required by the gateway service.
// It does not contain INI sections, open files, or device-library values.
type Config struct {
	Callsign string

	ReflectorName   string
	ReflectorModule string

	Duplex   bool
	AudioDir string
}

// Validate checks settings that do not require I/O.
func (c Config) Validate() error {
	if c.Callsign == "" {
		return ErrMissingCallsign
	}
	if len(c.ReflectorModule) > 1 {
		return ErrInvalidReflectorModule
	}
	return nil
}
