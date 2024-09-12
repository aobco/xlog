package main

import (
	"github.com/aobco/xlog"
	"time"
)

func main() {
	xlog.Init("demo/xxx.log").
		Level(xlog.TRACE).
		Hourly().
		Size(1, xlog.MB).
		Rotate(4).
		Compress(true)
	// defer func() {
	// 	xlog.Flush()
	// }()
	defer func() {
		if e := recover(); e != nil {
			xlog.Fatalf("%v", e)
		}
	}()

	for i := 1; i <= 100; i++ {
		time.Sleep(time.Millisecond)
		xlog.Tracef("==================== A%06d ", i)
		xlog.Debugf("==================== A%06d ", i)
		xlog.Infof("==================== A%06d ", i)
		xlog.Warnf("==================== A%06d ", i)
		xlog.Tracef("==================== A%06d ", i)
		xlog.Debugf("==================== A%06d ", i)
		xlog.Infof("==================== A%06d ", i)
		xlog.Warnf("==================== A%06d ", i)
		xlog.Errorf("==================== A%06d ", i)
		xlog.Panicf("==================== A%06d ", i)
	}
}
