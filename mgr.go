package xlog

import (
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type archiveFile struct {
	path        string
	size        int64
	archiveTime time.Time
}

func (l *logger) removeOlds() {
	cfg := l.getConfig()
	go func(cfg *loggerConfig) {
		if cfg.rotateByEnabled {
			l.removeOldArchivesByPolicy(cfg.maxArchiveSize, cfg.maxArchiveDays)
			return
		}
		glob, err := filepath.Glob(l.logFile + "*")
		if err != nil {
			keylog("%v", err)
			l.lastTime = time.Now()
			l.lastSeq = 0
			return
		}
		if len(glob) > cfg.rotateNo {
			sort.Strings(glob)
			for i := 1; i <= len(glob)-cfg.rotateNo; i++ {
				if err := os.Remove(glob[i]); err != nil {
					keylog("%v", err)
				}
			}
		}
	}(cfg)
}

func (l *logger) removeOldArchivesByPolicy(maxArchiveSize int64, maxArchiveDays int) {
	archives := l.listArchives()
	if maxArchiveDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -maxArchiveDays)
		remaining := archives[:0]
		for _, archive := range archives {
			if archive.archiveTime.Before(cutoff) {
				if err := os.Remove(archive.path); err != nil {
					keylog("%v", err)
					remaining = append(remaining, archive)
				}
				continue
			}
			remaining = append(remaining, archive)
		}
		archives = remaining
	}
	if maxArchiveSize <= 0 {
		return
	}
	var totalSize int64
	for _, archive := range archives {
		totalSize += archive.size
	}
	sort.Slice(archives, func(i, j int) bool {
		return archives[i].archiveTime.Before(archives[j].archiveTime)
	})
	for _, archive := range archives {
		if totalSize <= maxArchiveSize {
			return
		}
		if err := os.Remove(archive.path); err != nil {
			keylog("%v", err)
			continue
		}
		totalSize -= archive.size
	}
}

func (l *logger) listArchives() []archiveFile {
	glob, err := filepath.Glob(l.logFile + ".*")
	if err != nil {
		keylog("%v", err)
		return nil
	}
	archives := make([]archiveFile, 0, len(glob))
	for _, path := range glob {
		if path == l.logFile {
			continue
		}
		if !l.isArchive(path) {
			continue
		}
		stat, err := os.Stat(path)
		if err != nil {
			keylog("%v", err)
			continue
		}
		if stat.IsDir() {
			continue
		}
		archives = append(archives, archiveFile{path: path, size: stat.Size(), archiveTime: l.archiveTime(path, stat.ModTime())})
	}
	return archives
}

func (l *logger) isArchive(path string) bool {
	if strings.HasSuffix(path, ".gz") {
		return true
	}
	name := strings.TrimPrefix(path, l.logFile+".")
	parts := strings.Split(name, ".")
	if len(parts) != 2 {
		return false
	}
	if _, err := time.Parse(Minutely, parts[0]); err != nil {
		return false
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return false
	}
	return true
}

func (l *logger) archiveTime(path string, fallback time.Time) time.Time {
	name := strings.TrimPrefix(path, l.logFile+".")
	name = strings.TrimSuffix(name, ".gz")
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return fallback
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return fallback
	}
	archiveTime, err := time.Parse(Minutely, parts[0])
	if err != nil {
		return fallback
	}
	return archiveTime
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
