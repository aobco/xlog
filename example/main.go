package main

import (
	"github.com/aobco/xlog"
	"time"
)

func main() {
	log := xlog.Init("demo/xxx.log").
		Level(xlog.TRACE).
		Hourly().
		Size(1, xlog.MB).
		Rotate(4).
		Compress(true)
	defer log.Flush()

	defer func() {
		if e := recover(); e != nil {
			log.Fatalf("%v", e)
		}
	}()
	for i := 1; i <= 100; i++ {
		time.Sleep(time.Millisecond)
		log.Tracef("==================== A%06d ", i)
		log.Debugf("==================== A%06d ", i)
		log.Infof("==================== A%06d ", i)
		log.Warnf("==================== A%06d ", i)
		log.Tracef("==================== A%06d ", i)
		log.Debugf("==================== A%06d ", i)
		log.Infof("==================== A%06d ", i)
		log.Warnf("==================== A%06d ", i)
		log.Errorf("==================== A%06d ", i)
		log.Panicf("==================== A%06d ", i)
	}
}
