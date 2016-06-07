package healthchecker

type AggregateHealthChecker struct {
	HealthCheckers []HealthChecker
}

func (ahc AggregateHealthChecker) HealthCheck() error {

	for _, hc := range ahc.HealthCheckers {
		if err := hc.HealthCheck(); err != nil {
			return err
		}
	}

	return nil
}
