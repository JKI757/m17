package gateway

import "errors"

var (
	ErrMissingCallsign        = errors.New("gateway callsign is required")
	ErrInvalidReflectorModule = errors.New("gateway reflector module must contain zero or one character")
	ErrMissingModem           = errors.New("gateway modem is required")
)
