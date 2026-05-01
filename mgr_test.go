package xlog

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRotateByDeletesArchivesOlderThanMaxDays(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	oldArchive := archiveName(logFile, time.Now().AddDate(0, 0, -2))
	freshArchive := archiveName(logFile, time.Now())
	writeFile(t, oldArchive, 10)
	writeFile(t, freshArchive, 10)

	l := (&logger{logFile: logFile}).RotateBy(1, GB, 1)
	l.removeOlds()

	assertEventuallyMissing(t, oldArchive)
	assertExists(t, freshArchive)
}

func TestRotateByDeletesOldestArchivesUntilTotalSizeFits(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	oldArchive := archiveName(logFile, time.Now().Add(-2*time.Hour))
	newArchive := archiveName(logFile, time.Now().Add(-1*time.Hour))
	writeFile(t, oldArchive, 8)
	writeFile(t, newArchive, 8)

	l := (&logger{logFile: logFile}).RotateBy(10, B, 0)
	l.removeOlds()

	assertEventuallyMissing(t, oldArchive)
	assertExists(t, newArchive)
}

func TestRotateByDeletesUncompressedArchives(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	archiveTime := time.Now().AddDate(0, 0, -2)
	compressedArchive := archiveName(logFile, archiveTime)
	plainArchive := logFile + "." + archiveTime.Format(Minutely) + ".00"
	writeFile(t, compressedArchive, 8)
	writeFile(t, plainArchive, 8)
	setFileTime(t, plainArchive, archiveTime)

	l := (&logger{logFile: logFile}).RotateBy(1, GB, 1)
	l.removeOlds()

	assertEventuallyMissing(t, compressedArchive)
	assertEventuallyMissing(t, plainArchive)
}

func TestRotateByTakesPrecedenceOverRotate(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	firstArchive := archiveName(logFile, time.Now().Add(-2*time.Hour))
	secondArchive := archiveName(logFile, time.Now().Add(-1*time.Hour))
	writeFile(t, firstArchive, 4)
	writeFile(t, secondArchive, 4)

	l := (&logger{logFile: logFile}).Rotate(1).RotateBy(1, GB, 30)
	l.removeOlds()

	assertEventuallyExists(t, firstArchive)
	assertExists(t, secondArchive)
}

func TestRotateByUsesFilenameTimeBeforeModTime(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	archiveWithOldName := archiveName(logFile, time.Now().AddDate(0, 0, -2))
	archiveWithFreshName := archiveName(logFile, time.Now().AddDate(0, 0, 2))
	writeFile(t, archiveWithOldName, 8)
	writeFile(t, archiveWithFreshName, 8)
	setFileTime(t, archiveWithOldName, time.Now())
	setFileTime(t, archiveWithFreshName, time.Now().Add(-48*time.Hour))

	l := (&logger{logFile: logFile}).RotateBy(1, GB, 1)
	l.removeOlds()

	assertEventuallyMissing(t, archiveWithOldName)
	assertExists(t, archiveWithFreshName)
}

func TestRotateByFallsBackToModTimeWhenFilenameTimeIsInvalid(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	archiveWithBadName := logFile + ".bad.00.gz"
	writeFile(t, archiveWithBadName, 8)
	setFileTime(t, archiveWithBadName, time.Now().Add(-48*time.Hour))

	l := (&logger{logFile: logFile}).RotateBy(1, GB, 1)
	l.removeOlds()

	assertEventuallyMissing(t, archiveWithBadName)
}

func TestRotateKeepsOldCountRetentionWhenRotateByIsNotConfigured(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	activeLog := logFile
	firstArchive := archiveName(logFile, time.Now().Add(-3*time.Hour))
	secondArchive := archiveName(logFile, time.Now().Add(-2*time.Hour))
	thirdArchive := archiveName(logFile, time.Now().Add(-1*time.Hour))
	writeFile(t, activeLog, 4)
	writeFile(t, firstArchive, 4)
	writeFile(t, secondArchive, 4)
	writeFile(t, thirdArchive, 4)

	l := (&logger{logFile: logFile}).Rotate(2)
	l.removeOlds()

	assertEventuallyMissing(t, firstArchive)
	assertMissing(t, secondArchive)
	assertExists(t, thirdArchive)
}

func TestRotateByRuntimeUpdateUsesNewRetentionPolicy(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	oldArchive := archiveName(logFile, time.Now().AddDate(0, 0, -2))
	writeFile(t, oldArchive, 8)

	l := (&logger{logFile: logFile}).RotateBy(1, GB, 30)
	l.removeOlds()
	assertEventuallyExists(t, oldArchive)

	l.RotateBy(1, GB, 1)
	l.removeOlds()

	assertEventuallyMissing(t, oldArchive)
}

func TestFlushAllowsReinitWithoutSendingToClosedChannel(t *testing.T) {
	for i := 0; i < 3; i++ {
		dir := t.TempDir()
		Init(filepath.Join(dir, "app.log")).Level(INFO).Skip(3)
		Infof("before flush %d", i)
		Flush()
	}
}

func TestConcurrentFlushAndLoggingDoNotPanic(t *testing.T) {
	dir := t.TempDir()
	Init(filepath.Join(dir, "app.log")).Level(INFO).Skip(3)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				Infof("concurrent flush logger=%d seq=%d", id, j)
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		Flush()
	}()
	wg.Wait()
}

func TestSinkExitsAfterLogChannelCloses(t *testing.T) {
	dir := t.TempDir()
	l := &logger{
		logFile: filepath.Join(dir, "app.log"),
		logChan: make(chan string, 1),
		done:    make(chan interface{}),
	}
	l.config.Store(defaultLoggerConfig())
	go l.sink()

	close(l.logChan)
	select {
	case <-l.done:
	case <-time.After(time.Second):
		t.Fatal("expected sink to exit after log channel closes")
	}
	select {
	case <-l.done:
		t.Fatal("sink signaled done more than once")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestFlushAllowsImmediateReinitOnSamePath(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	for i := 0; i < 50; i++ {
		Init(logFile).Level(INFO).Skip(3)
		Infof("same path reinit %d", i)
		Flush()
	}
}

func TestRotateByIgnoresUncompressedFilesWithExtraSuffix(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	notArchive := logFile + "." + time.Now().AddDate(0, 0, -2).Format(Minutely) + ".00.backup"
	writeFile(t, notArchive, 8)
	setFileTime(t, notArchive, time.Now().AddDate(0, 0, -2))

	l := (&logger{logFile: logFile}).RotateBy(1, GB, 1)
	l.removeOlds()

	assertEventuallyExists(t, notArchive)
}

func TestRuntimeConfigUpdatesAreRaceFree(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	l := Init(logFile).Minutely().Size(1, MB).Rotate(2).RotateBy(1, GB, 30).Compress(true).Skip(3)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.Level(LoggerLevel(j % 5))
				l.Size(int64(j+1), MB)
				l.Rotate((j % 10) + 1)
				l.RotateBy(int64(j+1), MB, (j%7)+1)
				l.Compress(j%2 == 0)
				l.Skip((j % 5) + 1)
			}
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				Infof("runtime config update %d", j)
				l.removeOlds()
			}
		}()
	}
	wg.Wait()
	Flush()
	LOGGER = nil
}

func archiveName(logFile string, archiveTime time.Time) string {
	return logFile + "." + archiveTime.Format(Minutely) + ".00.gz"
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	data := make([]byte, size)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func setFileTime(t *testing.T, path string, mt time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mt, mt); err != nil {
		t.Fatalf("set file time %s: %v", path, err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, stat err: %v", path, err)
	}
}

func assertEventuallyExists(t *testing.T, path string) {
	t.Helper()
	assertEventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, "expected %s to exist", path)
}

func assertEventuallyMissing(t *testing.T, path string) {
	t.Helper()
	assertEventually(t, func() bool {
		_, err := os.Stat(path)
		return os.IsNotExist(err)
	}, "expected %s to be removed", path)
}

func assertEventually(t *testing.T, check func() bool, format string, args ...interface{}) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf(format, args...)
}
