package api

import (
	"time"
)

// Option is an additional argument to method New to change the behavior
// of API processing.
type Option interface {
	Apply(*Config)
}

// Config is a set of knobs to change the behavior of API processing.
// In other words, Config aggregates all the Option-s into one structure.
type Config struct {
	// EventTimeout defines time duration for API request to be processed.
	// If a request is not processed in time, then an error is returned to the
	// client.
	EventTimeout time.Duration

	// ServerIDFunc defines a custom server ID in API responses.
	ServerIDFunc ServerIDFunc
}

// OptionEventTimeout defines time duration for API request to be processed.
// If a request is not processed in time, then an error is returned to the
// client.
type OptionEventTimeout time.Duration

// Apply implements Option.
func (opt OptionEventTimeout) Apply(config *Config) {
	config.EventTimeout = (time.Duration)(opt)
}

// OptionServerID defines a custom server ID in API responses.
type OptionServerID string

// Apply implements Option.
func (opt OptionServerID) Apply(config *Config) {
	config.ServerIDFunc = func() string {
		return string(opt)
	}
}

// getConfig converts a set of Option-s into one structure "Config".
func getConfig(opts ...Option) Config {
	result := Config{
		EventTimeout: DefaultEventTimeout,
	}
	for _, opt := range opts {
		opt.Apply(&result)
	}
	return result
}
