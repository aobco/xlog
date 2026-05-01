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
	"sync/atomic"
	"time"
)

var LOGGER *logger
var globalLogger atomic.Value
var globalLoggerMu sync.Mutex
var stdoutLevel LoggerLevel = INFO

// SetStdoutLevel 设置未 Init 时 stdout 日志打印级别
func SetStdoutLevel(lvl LoggerLevel) {
	stdoutLevel = lvl
}

type loggerConfig struct {
	logLevel        LoggerLevel
	duration        string
	size            int64
	rotateNo        int
	rotateByEnabled bool
	maxArchiveSize  int64
	maxArchiveDays  int
	compress        bool
	stdout          bool
	skip            int
}

type logger struct {
	logFile     string
	config      atomic.Value
	configMu    sync.Mutex
	lifecycleMu sync.RWMutex
	closed      bool
	logChan     chan string
	done        chan interface{}
	fd          *os.File
	writer      *bufio.Writer
	lastTime    time.Time
	lastSeq     int
	lastSize    int64
	once        sync.Once
}

func Init(logFile string) *logger {
	keylog("init log %s", logFile)
	globalLoggerMu.Lock()
	defer globalLoggerMu.Unlock()
	if logger := getLogger(); logger != nil {
		keylog("duplicate log init, skip...")
		return logger
	}
	logger := &logger{
		logFile: logFile,
		logChan: make(chan string, 10240),
		done:    make(chan interface{}),
	}
	logger.config.Store(defaultLoggerConfig())
	setLogger(logger)
	go logger.sink()
	atExit(logger)
	return logger
}

func getLogger() *logger {
	if logger, ok := globalLogger.Load().(*logger); ok {
		return logger
	}
	return nil
}

func setLogger(logger *logger) {
	LOGGER = logger
	globalLogger.Store(logger)
}

func (l *logger) enqueue(msg string, dropIfFull bool) {
	l.lifecycleMu.RLock()
	defer l.lifecycleMu.RUnlock()
	if l.closed {
		return
	}
	if dropIfFull {
		select {
		case l.logChan <- msg:
		default:
		}
		return
	}
	l.logChan <- msg
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
				return
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						keylog("%v", r)
						l.reload()
					}
				}()
				if l.getConfig().stdout {
					fmt.Print(msg)
				}
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

func defaultLoggerConfig() *loggerConfig {
	return &loggerConfig{
		rotateNo: 100,
		skip:     3,
	}
}

func (l *logger) getConfig() *loggerConfig {
	if l == nil {
		return defaultLoggerConfig()
	}
	if cfg, ok := l.config.Load().(*loggerConfig); ok && cfg != nil {
		return cfg
	}
	return defaultLoggerConfig()
}

func (l *logger) updateConfig(update func(*loggerConfig)) *logger {
	if l == nil {
		return l
	}
	l.configMu.Lock()
	defer l.configMu.Unlock()
	old := l.getConfig()
	next := *old
	update(&next)
	l.config.Store(&next)
	return l
}

func (l *logger) Level(lvl LoggerLevel) *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.logLevel = lvl
	})
}

func (l *logger) Minutely() *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.duration = Minutely
	})
}

func (l *logger) Hourly() *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.duration = Hourly
	})
}

func (l *logger) Daily() *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.duration = Daily
	})
}

func (l *logger) Monthly() *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.duration = Monthly
	})
}

func (l *logger) Yearly() *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.duration = Yearly
	})
}

func (l *logger) Stdout() *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.stdout = true
	})
}
func (l *logger) Skip(skip int) *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.skip = skip
	})
}

func (l *logger) Size(size int64, unit SizeUnit) *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.size = size * int64(unit)
	})
}

func (l *logger) Rotate(rotate int) *logger {
	if rotate < 1 {
		rotate = 1
	}
	if rotate > 100 {
		rotate = 100
	}
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.rotateNo = rotate
	})
}

func (l *logger) RotateBy(maxTotalSize int64, unit SizeUnit, maxDays int) *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.rotateByEnabled = true
		if maxTotalSize > 0 {
			cfg.maxArchiveSize = maxTotalSize * int64(unit)
		}
		cfg.maxArchiveDays = maxDays
	})
}

func (l *logger) Compress(compress bool) *logger {
	return l.updateConfig(func(cfg *loggerConfig) {
		cfg.compress = compress
	})
}

func Flush() {
	globalLoggerMu.Lock()
	defer globalLoggerMu.Unlock()
	logger := getLogger()
	if logger == nil {
		return
	}
	logger.once.Do(func() {
		logger.lifecycleMu.Lock()
		logger.closed = true
		close(logger.logChan)
		logger.lifecycleMu.Unlock()
		<-logger.done
		logger.closeFile()
		setLogger(nil)
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
	cfg := l.getConfig()
	if len(cfg.duration) > 0 && time.Now().Format(cfg.duration) != l.lastTime.Format(cfg.duration) {
		l.rotate(time.Now(), 0)
		return true
	}
	if cfg.size > 0 {
		if l.lastSize > cfg.size {
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
		if err = os.Rename(l.logFile, tmpLog); err != nil {
			keylog("%v", err)
			time.Sleep(time.Second)
			continue
		}
		break
	}
	if err != nil {
		return
	}
	for i := 0; i < 5; i++ {
		stat, err := os.Stat(tmpLog)
		if err != nil {
			keylog("%v", err)
			time.Sleep(time.Second)
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
	compress := l.getConfig().compress
	go func() {
		if compress {
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
	skip := 3
	if logger := getLogger(); logger != nil {
		skip = logger.getConfig().skip
	}
	fmt.Printf("%s\t%s\t%s\t%s\n", time.Now().Format(time.RFC3339), lvl, caller(skip), fmt.Sprintf(format, args...))
}

func Tracef(format string, args ...interface{}) {
	logger := getLogger()
	if logger == nil {
		if stdoutLevel > TRACE {
			return
		}
		stdoutf("TRACE", format, args...)
		return
	}
	cfg := logger.getConfig()
	if cfg.logLevel > TRACE {
		return
	}
	log := msgWithSkip(false, "TRACE", cfg.skip, format, args...)
	logger.enqueue(log, true)
}

func Debugf(format string, args ...interface{}) {
	logger := getLogger()
	if logger == nil {
		if stdoutLevel > DEBUG {
			return
		}
		stdoutf("DEBUG", format, args...)
		return
	}
	cfg := logger.getConfig()
	if cfg.logLevel > DEBUG {
		return
	}
	log := msgWithSkip(false, "DEBUG", cfg.skip, format, args...)
	logger.enqueue(log, false)
}

func Infof(format string, args ...interface{}) {
	logger := getLogger()
	if logger == nil {
		if stdoutLevel > INFO {
			return
		}
		stdoutf("INFO", format, args...)
		return
	}
	cfg := logger.getConfig()
	if cfg.logLevel > INFO {
		return
	}
	log := msgWithSkip(false, "INFO", cfg.skip, format, args...)
	logger.enqueue(log, false)
}

func Warnf(format string, args ...interface{}) {
	logger := getLogger()
	if logger == nil {
		if stdoutLevel > WARN {
			return
		}
		stdoutf("WARN", format, args...)
		return
	}
	cfg := logger.getConfig()
	if cfg.logLevel > WARN {
		return
	}
	log := msgWithSkip(false, "WARN", cfg.skip, format, args...)
	logger.enqueue(log, false)
}

func Errorf(format string, args ...interface{}) {
	logger := getLogger()
	if logger == nil {
		if stdoutLevel > ERROR {
			return
		}
		stdoutf("ERROR", format, args...)
		return
	}
	cfg := logger.getConfig()
	if cfg.logLevel > ERROR {
		return
	}
	logger.enqueue(msgWithSkip(true, "ERROR", cfg.skip, format, args...), false)
}

func Panicf(format string, args ...interface{}) {
	logger := getLogger()
	if logger == nil {
		if stdoutLevel <= PANIC {
			stdoutf("PANIC", format, args...)
		}
		panic(errors.New(fmt.Sprintf(format, args...)))
	}
	m := fmt.Sprintf(format, args...)
	cfg := logger.getConfig()
	if cfg.logLevel <= PANIC {
		logger.enqueue(msgWithSkip(true, "PANIC", cfg.skip, m), false)
	}
	panic(errors.New(m))
}

func Fatalf(format string, args ...interface{}) {
	logger := getLogger()
	if logger == nil {
		if stdoutLevel <= FATAL {
			stdoutf("FATAL", format, args...)
		}
		os.Exit(1)
	}
	cfg := logger.getConfig()
	if cfg.logLevel > FATAL {
		return
	}
	logger.enqueue(msgWithSkip(true, "FATAL", cfg.skip, format, args...), false)
	Flush()
	os.Exit(1)
}

func msg(trace bool, lvl, format string, args ...interface{}) string {
	skip := 3
	if logger := getLogger(); logger != nil {
		skip = logger.getConfig().skip
	}
	return msgWithSkip(trace, lvl, skip, format, args...)
}

func msgWithSkip(trace bool, lvl string, skip int, format string, args ...interface{}) string {
	log := fmt.Sprintf(format, args...)
	buf := bufferpool.Get()
	buf.AppendString(time.Now().Format(time.RFC3339))
	buf.AppendByte('\t')
	buf.AppendString(lvl)
	buf.AppendByte('\t')
	buf.AppendString(caller(skip))
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
