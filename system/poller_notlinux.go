// +build !linux

package system

func (p *Poller) Poll() error {
	return nil
}

func NewPoller(fds []uintptr) *Poller {
	return nil
}
