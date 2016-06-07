package healthchecker

type NetworkAllocationHealthChecker struct {
	Conflict error
}

func (nahc *NetworkAllocationHealthChecker) ConflictDetected(err error) {
	nahc.Conflict = err
}

func (nahc *NetworkAllocationHealthChecker) HealthCheck() error {
	return nahc.Conflict
}
