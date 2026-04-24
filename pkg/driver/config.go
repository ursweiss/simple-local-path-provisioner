package driver

import "time"

// Config holds all driver configuration.
type Config struct {
	DriverName         string
	Endpoint           string
	NodeID             string
	BasePath           string
	Mode               string
	StaleTimeout       time.Duration
	AllowForceTakeover bool
	LogLevel           int
}
