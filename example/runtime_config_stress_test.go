package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aobco/xlog"
)

const (
	stressDiskLimit      int64 = 1 << 30
	stressWriters              = 16
	stressLinesPerWriter       = 8000
)

func TestHighFrequencyLoggingWithRotateByHotUpdates(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "stress.log")
	logger := xlog.Init(logFile).
		Level(xlog.INFO).
		Size(8, xlog.MB).
		RotateBy(256, xlog.KB, 1).
		Compress(true).
		Skip(3)

	start := time.Now()
	var wg sync.WaitGroup
	for writer := 0; writer < stressWriters; writer++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for i := 0; i < stressLinesPerWriter; i++ {
				xlog.Infof("writer=%02d seq=%05d payload=abcdefghijklmnopqrstuvwxyz0123456789", writer, i)
			}
		}(writer)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < stressWriters*stressLinesPerWriter; i++ {
			if i%2 == 0 {
				logger.RotateBy(256, xlog.KB, 1)
				continue
			}
			logger.RotateBy(768, xlog.KB, 30)
		}
	}()

	wg.Wait()
	xlog.Flush()

	usage := dirSize(t, dir)
	if usage > stressDiskLimit {
		t.Fatalf("stress test used %d bytes, exceeds %d byte limit", usage, stressDiskLimit)
	}
	t.Logf("wrote %d log calls with %d RotateBy updates in %s, disk usage=%d bytes", stressWriters*stressLinesPerWriter, stressWriters*stressLinesPerWriter, time.Since(start), usage)
}

func TestRotateByRetentionPolicyChangesAtRuntime(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "retention.log")
	oldArchive := archivePath(logFile, time.Now().AddDate(0, 0, -2), 0)
	freshArchive := archivePath(logFile, time.Now(), 0)
	writeBytes(t, oldArchive, 128*int(xlog.KB))
	writeBytes(t, freshArchive, 128*int(xlog.KB))

	logger := xlog.Init(logFile).
		Level(xlog.INFO).
		Size(4, xlog.KB).
		RotateBy(1, xlog.MB, 30).
		Compress(false).
		Skip(3)

	writeUntilRotated(t, oldArchive, func(i int) {
		xlog.Infof("loose seq=%04d payload=abcdefghijklmnopqrstuvwxyz0123456789", i)
	})
	assertExists(t, oldArchive)

	logger.RotateBy(256, xlog.KB, 1)
	writeUntilRemoved(t, oldArchive, func(i int) {
		xlog.Infof("strict seq=%04d payload=abcdefghijklmnopqrstuvwxyz0123456789", i)
	})
	assertExists(t, freshArchive)
	xlog.Flush()

	usage := dirSize(t, dir)
	if usage > stressDiskLimit {
		t.Fatalf("runtime retention test used %d bytes, exceeds %d byte limit", usage, stressDiskLimit)
	}
}

func BenchmarkInfofBaseline(b *testing.B) {
	benchmarkInfof(b, false)
}

func BenchmarkInfofWithRotateByHotUpdates(b *testing.B) {
	benchmarkInfof(b, true)
}

func benchmarkInfof(b *testing.B, hotUpdate bool) {
	dir := b.TempDir()
	logFile := filepath.Join(dir, "bench.log")
	logger := xlog.Init(logFile).
		Level(xlog.INFO).
		Size(64, xlog.MB).
		RotateBy(1, xlog.MB, 1).
		Compress(false).
		Skip(3)
	defer func() {
		xlog.Flush()
		usage := dirSizeForBenchmark(b, dir)
		if usage > stressDiskLimit {
			b.Fatalf("benchmark used %d bytes, exceeds %d byte limit", usage, stressDiskLimit)
		}
	}()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	if hotUpdate {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				if i%2 == 0 {
					logger.RotateBy(512, xlog.KB, 1)
					continue
				}
				logger.RotateBy(2, xlog.MB, 30)
			}
		}()
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		xlog.Infof("bench seq=%08d payload=abcdefghijklmnopqrstuvwxyz0123456789", i)
	}
	b.StopTimer()
	close(stop)
	wg.Wait()
}

func writeUntilRotated(t *testing.T, existingArchive string, write func(int)) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for i := 0; time.Now().Before(deadline); i++ {
		write(i)
		if i%64 != 0 {
			continue
		}
		if rotatedArchiveCount(t, filepath.Dir(existingArchive)) > 2 {
			return
		}
	}
	t.Fatalf("expected log rotation before deadline")
}

func writeUntilRemoved(t *testing.T, path string, write func(int)) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for i := 0; time.Now().Before(deadline); i++ {
		write(i)
		if i%64 != 0 {
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
	}
	t.Fatalf("expected %s to be removed after runtime RotateBy update", path)
}

func rotatedArchiveCount(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}
	return count
}

func archivePath(logFile string, archiveTime time.Time, seq int) string {
	return fmt.Sprintf("%s.%s.%02d.gz", logFile, archiveTime.Format(xlog.Minutely), seq)
}

func writeBytes(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func dirSize(t *testing.T, root string) int64 {
	t.Helper()
	return walkDirSize(t, root)
}

func dirSizeForBenchmark(b *testing.B, root string) int64 {
	b.Helper()
	var total int64
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	}); err != nil {
		b.Fatalf("walk %s: %v", root, err)
	}
	return total
}

func walkDirSize(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return total
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertEventuallyMissing(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %s to be removed", path)
}
