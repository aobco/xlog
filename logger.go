package xlog

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"github.com/aobco/xlog/bufferpool"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var LOGGER *logger

type logger struct {
	logFile  string
	logLevel LoggerLevel
	duration string
	size     int64
	rotateNo int
	compress bool
	logChan  chan string
	done     chan interface{}
	fd       *os.File
	writer   *bufio.Writer
	lastTime time.Time
	lastSeq  int
	lastSize int64
	once     sync.Once
}

func Init(logFile string) *logger {
	keylog("init log %s", logFile)
	if LOGGER != nil {
		keylog("duplicate log init, skip...")
		return LOGGER
	}
	LOGGER = &logger{
		logFile:  logFile,
		rotateNo: 100,
		logChan:  make(chan string, 10240),
		done:     make(chan interface{}),
	}
	go LOGGER.sink()
	atExit(LOGGER)
	return LOGGER
}

func (l *logger) sink() {
	lastSync := time.Now()
	lastLoad := time.Now()
	l.checkFile()
	stat, err := l.fd.Stat()
	if err != nil {
		keylog("%v", err)
		return
	}
	l.lastSize = stat.Size()
	l.removeOlds()
	for {
		select {
		case msg, ok := <-l.logChan:
			if !ok {
				l.done <- struct{}{}
				break
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						keylog("%v", err)
						l.reload()
					}
				}()
				fmt.Print(msg)
				n, err := l.writer.WriteString(msg)
				if err != nil {
					keylog("write log %v", err)
					l.reload()
					l.writer.WriteString(msg)
					return
				}
				l.lastSize = l.lastSize + int64(n)
				l.checkFile()
				if time.Since(lastSync) > time.Second {
					lastSync = time.Now()
					l.writer.Flush()
				}
				if time.Since(lastLoad) > 10*time.Second {
					lastLoad = time.Now()
					l.reload()
				}
			}()
		}
	}
}

func (l *logger) Level(lvl LoggerLevel) *logger {
	l.logLevel = lvl
	return l
}

func (l *logger) Minutely() *logger {
	l.duration = Minutely
	return l
}

func (l *logger) Hourly() *logger {
	l.duration = Hourly
	return l
}

func (l *logger) Daily() *logger {
	l.duration = Daily
	return l
}

func (l *logger) Monthly() *logger {
	l.duration = Monthly
	return l
}
func (l *logger) Yearly() *logger {
	l.duration = Yearly
	return l
}

func (l *logger) Size(size int64, unit SizeUnit) *logger {
	l.size = size * int64(unit)
	return l
}

func (l *logger) Rotate(rotate int) *logger {
	if rotate < 1 {
		rotate = 1
	}
	if rotate > 100 {
		rotate = 100
	}
	l.rotateNo = rotate
	return l
}

func (l *logger) Compress(compress bool) *logger {
	l.compress = compress
	return l
}

func Flush() {
	if LOGGER == nil {
		return
	}
	LOGGER.once.Do(func() {
		close(LOGGER.logChan)
		<-LOGGER.done
		LOGGER.closeFile()
	})
}

func (l *logger) closeFile() {
	if l.writer != nil {
		if err := l.writer.Flush(); err != nil {
			keylog("%v", err)
		}
		l.writer = nil
	}
	if l.fd != nil {
		if err := l.fd.Sync(); err != nil {
			keylog("%v", err)
		}
		if err := l.fd.Close(); err != nil {
			keylog("%v", err)
		}
		l.fd = nil
	}
}

func (l *logger) checkFile() bool {
	if l.writer == nil || l.fd == nil {
		l.reload()
		l.refreshLastTime()
	}
	if len(l.duration) > 0 && time.Now().Format(l.duration) != l.lastTime.Format(l.duration) {
		l.rotate(time.Now(), 0)
		return true
	}
	if l.size > 0 {
		if l.lastSize > l.size {
			l.rotate(time.Now(), l.lastSeq+1)
			return true
		}
	}
	return false
}

func (l *logger) reload() {
	l.closeFile()
	fd, err := os.OpenFile(l.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		keylog("%v", err)
		return
	}
	l.fd = fd
	l.writer = bufio.NewWriter(fd)
}

func (l *logger) rotate(dt time.Time, seq int) {
	seq = seq % 100
	l.closeFile()
	tmpLog := fmt.Sprintf("%s.%s.%02d", l.logFile, dt.Format(Minutely), seq)
	var err error
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second)
		if err = os.Rename(l.logFile, tmpLog); err != nil {
			keylog("%v", err)
			continue
		}
		break
	}
	if err != nil {
		return
	}
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second)
		stat, err := os.Stat(tmpLog)
		if err != nil {
			keylog("%v", err)
			continue
		}
		if stat != nil {
			break
		}
	}
	os.Remove(l.logFile)
	l.reload()
	l.lastTime = dt
	l.lastSeq = seq
	l.lastSize = 0
	go func() {
		if l.compress {
			file, err := os.Create(fmt.Sprintf("%s.%s.%02d.gz", l.logFile, dt.Format(Minutely), seq))
			if err != nil {
				keylog("%v", err)
				return
			}
			defer file.Close()
			zipWriter := zip.NewWriter(file)
			defer zipWriter.Close()
			addFileToZip(tmpLog, zipWriter)
			if err = os.Remove(tmpLog); err != nil {
				keylog("%v", err)
				return
			}
		}
		l.removeOlds()
	}()
}

func (l *logger) refreshLastTime() {
	glob, err := filepath.Glob(l.logFile + "*")
	if err != nil {
		keylog("%v", err)
		l.lastTime = time.Now()
		l.lastSeq = 0
		return
	}
	if len(glob) == 0 {
		l.lastTime = time.Now()
		l.lastSeq = 0
		return
	}
	sort.Strings(glob)
	max := glob[len(glob)-1]
	max = strings.TrimSuffix(max, l.logFile)
	split := strings.Split(max, ".")
	if len(split) < 3 {
		l.lastTime = time.Now()
		l.lastSeq = 0
		return
	}
	l.lastTime, err = time.Parse(Minutely, split[1])
	if err != nil {
		keylog("%v", err)
		l.lastTime = time.Now()
		l.lastSeq = 0
		return
	}
	l.lastSeq, err = strconv.Atoi(split[2])
	if err != nil {
		keylog("%v", err)
		l.lastTime = time.Now()
		l.lastSeq = 0
		return
	}
}

func stdoutf(lvl string, format string, args ...interface{}) {
	fmt.Printf("%s\t%s\t%s\t%s\n", time.Now().Format(time.RFC3339), lvl, caller(3), fmt.Sprintf(format, args...))
}

func Tracef(format string, args ...interface{}) {
	if LOGGER == nil {
		stdoutf("TRACE", format, args...)
		return
	}
	if LOGGER.logLevel > TRACE {
		return
	}
	log := msg(false, "TRACE", format, args...)
	LOGGER.logChan <- log
}

func Debugf(format string, args ...interface{}) {
	if LOGGER == nil {
		stdoutf("DEBUG", format, args...)
		return
	}
	if LOGGER.logLevel > DEBUG {
		return
	}
	log := msg(false, "DEBUG", format, args...)
	LOGGER.logChan <- log
}

func Infof(format string, args ...interface{}) {
	if LOGGER == nil {
		stdoutf("INFO", format, args...)
		return
	}
	if LOGGER.logLevel > INFO {
		return
	}
	log := msg(false, "INFO", format, args...)
	LOGGER.logChan <- log
}

func Warnf(format string, args ...interface{}) {
	if LOGGER == nil {
		stdoutf("WARN", format, args...)
		return
	}
	if LOGGER.logLevel > WARN {
		return
	}
	log := msg(false, "WARN", format, args...)
	LOGGER.logChan <- log
}

func Errorf(format string, args ...interface{}) {
	if LOGGER == nil {
		stdoutf("ERROR", format, args...)
		return
	}
	if LOGGER.logLevel > ERROR {
		return
	}
	LOGGER.logChan <- msg(true, "ERROR", format, args...)
}

func Panicf(format string, args ...interface{}) {
	if LOGGER == nil {
		stdoutf("PANIC", format, args...)
		return
	}
	m := fmt.Sprintf(format, args...)
	if LOGGER.logLevel <= PANIC {
		LOGGER.logChan <- msg(true, "PANIC", m)
	}
	panic(errors.New(m))
}

func Fatalf(format string, args ...interface{}) {
	if LOGGER == nil {
		stdoutf("FATAL", format, args...)
		return
	}
	if LOGGER.logLevel > FATAL {
		return
	}
	LOGGER.logChan <- msg(true, "FATAL", format, args...)
	Flush()
	os.Exit(1)
}

func msg(trace bool, lvl, format string, args ...interface{}) string {
	log := fmt.Sprintf(format, args...)
	buf := bufferpool.Get()
	buf.AppendString(time.Now().Format(time.RFC3339))
	buf.AppendByte('\t')
	buf.AppendString(lvl)
	buf.AppendByte('\t')
	buf.AppendString(caller(3))
	buf.AppendByte('\t')
	buf.AppendString(log)
	buf.AppendByte('\n')
	if trace {
		buf.AppendString(stackTrace())
	}
	log = buf.String()
	buf.Free()
	return log
}
