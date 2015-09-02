package layercake

//go:generate counterfeiter -o fake_retainer/fake_retainer.go . Retainer
type Retainer interface {
	Retain(id ID)
	Release(id ID)
	IsHeld(id ID) bool
}
