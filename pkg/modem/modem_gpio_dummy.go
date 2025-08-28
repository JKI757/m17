//go:build !linux

package modem

func (m *CC1200Modem) gpioSetup(_, _, _ int) error {
	return nil
}
