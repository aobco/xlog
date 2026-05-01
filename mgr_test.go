package xlog

import (
	"os"
	"path/filepath"
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

func TestRotateByOnlyDeletesGzArchives(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	archiveTime := time.Now().AddDate(0, 0, -2)
	archive := archiveName(logFile, archiveTime)
	plainRotated := logFile + "." + archiveTime.Format(Minutely) + ".00"
	writeFile(t, archive, 8)
	writeFile(t, plainRotated, 8)
	setFileTime(t, plainRotated, archiveTime)

	l := (&logger{logFile: logFile}).RotateBy(1, GB, 1)
	l.removeOlds()

	assertEventuallyMissing(t, archive)
	assertExists(t, plainRotated)
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
