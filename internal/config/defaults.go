package config

import "time"

const (
	Host                      = "localhost"
	Port                      = 8080
	PollInterval              = 2
	ReportInterval            = 10
	StoreInterval             = 300
	FileStoragePath           = "data/metrics.json"
	Restore                   = true
	ServerShutdownGracePeriod = 1
	DBConnectTimeout          = 2 * time.Second
	DBQueryTimeout            = 2 * time.Second
)
