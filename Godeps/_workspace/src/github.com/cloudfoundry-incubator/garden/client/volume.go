package client

type volume struct {
	handle string
}

func newVolume(handle string) *volume {
	return &volume{handle: handle}
}

func (volume *volume) Handle() string {
	return volume.handle
}
