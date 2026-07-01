package config

import "time"

// ExternalReadinessProbe configures an optional background dependency check that
// contributes to this application's readiness state.
type ExternalReadinessProbe struct {
	Type               string
	Target             string
	Interval           time.Duration
	Timeout            time.Duration
	HTTPMethod         string
	HTTPExpectedStatus int
}

// Enabled reports whether an external readiness probe is configured.
func (p ExternalReadinessProbe) Enabled() bool {
	return p.Type != "" && p.Type != "none" && p.Target != ""
}
