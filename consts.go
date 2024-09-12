package xlog

const (
	Minutely = "200601021504"
	Hourly   = "2006010215"
	Daily    = "20060102"
	Monthly  = "200601"
	Yearly   = "2006"
)

type LoggerLevel int

const (
	TRACE LoggerLevel = 0
	DEBUG LoggerLevel = 10
	INFO  LoggerLevel = 20
	WARN  LoggerLevel = 30
	ERROR LoggerLevel = 40
	PANIC LoggerLevel = 50
	FATAL LoggerLevel = 60
)

type SizeUnit int64

const (
	B  = 1
	KB = 1024 * B
	MB = 1024 * KB
	GB = 1024 * MB
	TB = 1024 * GB
	PB = 1024 * TB
)
