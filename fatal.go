package xlog

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const filePath = "fatal.log"

var (
	file  *os.File
	pid   int
	pName string
)

func keylog(format string, msg ...interface{}) {
	var err error
	if file == nil {
		file, err = os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		pid = os.Getpid()
		pName, _ = os.Executable()
		pName = filepath.Base(pName)
	}
	file.WriteString(fmt.Sprintf("%s\t[%d]%s\t%s\t%s\n", time.Now().Format(time.RFC3339), pid, pName, caller(2), fmt.Sprintf(format, msg...)))
	file.Sync()
}
