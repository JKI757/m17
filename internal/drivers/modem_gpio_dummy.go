//go:build !linux

package drivers

func (m *CC1200Modem) gpioSetup(_, _ int) error {
	return nil
}
