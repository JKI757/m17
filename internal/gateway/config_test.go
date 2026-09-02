package gateway

import (
	"errors"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	if err := (Config{}).Validate(); err != ErrMissingCallsign {
		t.Fatalf("Validate() error = %v, want %v", err, ErrMissingCallsign)
	}
	if err := (Config{Callsign: "N0CALL", ReflectorModule: "AB"}).Validate(); err != ErrInvalidReflectorModule {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidReflectorModule)
	}
	if err := (Config{Callsign: "N0CALL", ReflectorModule: "A"}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestNewRequiresModem(t *testing.T) {
	_, err := New(Config{Callsign: "N0CALL"}, Dependencies{})
	if !errors.Is(err, ErrMissingModem) {
		t.Fatalf("New() error = %v, want %v", err, ErrMissingModem)
	}
}
