package xlog

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

func stackTrace() string {
	messages := string(debug.Stack())
	split := strings.Split(messages, "\n")
	if len(split) > 5 {
		split = append(split[0:1], split[7:]...)
		messages = strings.Join(split, "\n")
	}
	return messages
}

func caller() string {
	_, file, line, ok := runtime.Caller(2)
	if ok {
		idx := strings.LastIndexByte(file, '/')
		if idx == -1 {
			return fmt.Sprintf("%s:%d", file, line)
		}
		idx = strings.LastIndexByte(file[:idx], '/')
		if idx == -1 {
			return fmt.Sprintf("%s:%d", file, line)
		}
		return fmt.Sprintf("%s:%d", file[idx+1:], line)
	}
	return "unknown:NaN"
}
