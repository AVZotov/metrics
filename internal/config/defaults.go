package config

import "time"

const (
	host                      = "localhost"
	port                      = 8080
	pollInterval              = 2
	reportInterval            = 10
	rateLimit                 = 1
	storeInterval             = 300
	fileStoragePath           = "data/metrics.json"
	restore                   = true
	enablePprof               = false
	serverShutdownGracePeriod = 1
	dbConnectTimeout          = 2 * time.Second
	dbQueryTimeout            = 2 * time.Second
)
