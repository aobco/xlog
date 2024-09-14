package xlog

import (
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"time"
)

func (l *logger) removeOlds() {
	go func() {
		glob, err := filepath.Glob(l.logFile + "*")
		if err != nil {
			keylog("%v", err)
			l.lastTime = time.Now()
			l.lastSeq = 0
			return
		}
		if len(glob) > l.rotateNo {
			sort.Strings(glob)
			for i := 1; i <= len(glob)-l.rotateNo; i++ {
				if err := os.Remove(glob[i]); err != nil {
					keylog("%v", err)
				}
			}
		}
	}()
}

func atExit(l *logger) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		keylog("Received signal %s, exiting...", sig)
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		keylog("=== Stack Trace ===\n%s", string(buf[:n]))
		Flush()
	}()
}
