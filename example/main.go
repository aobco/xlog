package main

import (
	"github.com/aobco/xlog"
	"os"
	"sync"
	"time"
)

func main() {
	os.Remove("demo/xxx.log")
	xlog.Init("demo/xxx.log").
		Level(xlog.TRACE).
		Hourly().
		Size(1, xlog.MB).
		Rotate(4).
		Compress(true)
	defer xlog.Flush()
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 1; i <= 100; i++ {
			time.Sleep(time.Millisecond)
			xlog.Tracef("==================== A%06d ", i)
			xlog.Tracef("==================== A%06d ", i)
			xlog.Debugf("==================== A%06d ", i)
			xlog.Infof("==================== A%06d ", i)
			xlog.Warnf("==================== A%06d ", i)
			xlog.Tracef("==================== A%06d ", i)
			xlog.Debugf("==================== A%06d ", i)
			xlog.Infof("==================== A%06d ", i)
			xlog.Warnf("==================== A%06d ", i)
			xlog.Errorf("==================== A%06d ", i)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 1; i <= 100; i++ {
			time.Sleep(time.Millisecond)
			xlog.Tracef("++++++++++++++++++++ A%06d ", i)
			xlog.Tracef("++++++++++++++++++++ A%06d ", i)
			xlog.Debugf("++++++++++++++++++++ A%06d ", i)
			xlog.Infof("++++++++++++++++++++ A%06d ", i)
			xlog.Warnf("++++++++++++++++++++ A%06d ", i)
			xlog.Tracef("++++++++++++++++++++ A%06d ", i)
			xlog.Debugf("++++++++++++++++++++ A%06d ", i)
			xlog.Infof("++++++++++++++++++++ A%06d ", i)
			xlog.Warnf("++++++++++++++++++++ A%06d ", i)
			xlog.Errorf("++++++++++++++++++++ A%06d ", i)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 1; i <= 100; i++ {
			time.Sleep(time.Millisecond)
			xlog.Tracef("---------------------------- A%06d ", i)
			xlog.Tracef("---------------------------- A%06d ", i)
			xlog.Debugf("---------------------------- A%06d ", i)
			xlog.Infof("---------------------------- A%06d ", i)
			xlog.Warnf("---------------------------- A%06d ", i)
			xlog.Tracef("---------------------------- A%06d ", i)
			xlog.Debugf("---------------------------- A%06d ", i)
			xlog.Infof("---------------------------- A%06d ", i)
			xlog.Warnf("---------------------------- A%06d ", i)
			xlog.Errorf("---------------------------- A%06d ", i)
		}
	}()
	wg.Wait()
	println("TEST DONE")
}
