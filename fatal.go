package xlog

import (
	"fmt"
	"os"
)

const filePath = "fatal.log"

var file *os.File

func ff(msg string) {
	var err error
	file, err = os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	file.WriteString(fmt.Sprintf("%s\n", msg))
	file.Sync()
}
