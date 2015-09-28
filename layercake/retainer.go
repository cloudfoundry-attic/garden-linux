package layercake

//go:generate counterfeiter -o fake_id_provider/fake_id_provider.go . IDProvider
type IDProvider interface {
	ProvideID(path string) (ID, error)
}

//go:generate counterfeiter -o fake_retainer/fake_retainer.go . Retainer
type Retainer interface {
	Retain(id ID)
}
