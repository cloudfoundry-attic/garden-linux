package healthchecker

//go:generate counterfeiter . HealthChecker

type HealthChecker interface {
	HealthCheck() error
}
