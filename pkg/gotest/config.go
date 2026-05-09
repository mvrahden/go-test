package gotest

import "time"

type FixtureConfig struct {
	Timeout    time.Duration
	Retries    int
	RetryDelay time.Duration
}

type SuiteConfig struct {
	Timeout      time.Duration
	SetupTimeout time.Duration
	Retries      int
	FailFast     bool
	Parallel     bool
	Sequential   bool
}

func DefaultFixtureConfig() FixtureConfig {
	return FixtureConfig{Timeout: 2 * time.Minute}
}

func ContainerFixtureConfig() FixtureConfig {
	return FixtureConfig{Timeout: 5 * time.Minute, Retries: 1, RetryDelay: 5 * time.Second}
}

func DefaultSuiteConfig() SuiteConfig {
	return SuiteConfig{Timeout: 30 * time.Second, SetupTimeout: 30 * time.Second}
}

func IntegrationSuiteConfig() SuiteConfig {
	return SuiteConfig{Timeout: 2 * time.Minute, SetupTimeout: 5 * time.Minute}
}

func OverlayFixtureConfig(base *FixtureConfig, overlay FixtureConfig) {
	if overlay.Timeout != 0 {
		base.Timeout = overlay.Timeout
	}
	if overlay.Retries != 0 {
		base.Retries = overlay.Retries
	}
	if overlay.RetryDelay != 0 {
		base.RetryDelay = overlay.RetryDelay
	}
}

func OverlaySuiteConfig(base *SuiteConfig, overlay SuiteConfig) {
	if overlay.Timeout != 0 {
		base.Timeout = overlay.Timeout
	}
	if overlay.SetupTimeout != 0 {
		base.SetupTimeout = overlay.SetupTimeout
	}
	if overlay.Retries != 0 {
		base.Retries = overlay.Retries
	}
	if overlay.FailFast {
		base.FailFast = true
	}
	if overlay.Parallel {
		base.Parallel = true
	}
	if overlay.Sequential {
		base.Sequential = true
	}
}
