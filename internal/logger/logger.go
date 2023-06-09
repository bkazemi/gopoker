package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Logger struct {
	logger *log.Logger
	prefix string
}

func NewLogger(prefix string) *Logger {
	return &Logger{
		logger: log.New(os.Stdout, "", log.Ldate|log.Ltime),
		prefix: prefix,
	}
}

func (l *Logger) Logf(format string, args ...interface{}) {
	l.logWithPrefix(fmt.Sprintf(format, args...))
}

func (l *Logger) Logln(args ...interface{}) {
	l.logWithPrefix(fmt.Sprintln(args...))
}

func (l *Logger) logWithPrefix(msg string) {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		return
	}

	funcName := runtime.FuncForPC(pc).Name()
	lastSlash := strings.LastIndexByte(funcName, '/')
	funcName = funcName[lastSlash+1:]

	_, fileName := filepath.Split(file)

	prefix := fmt.Sprintf("%s [%s:%d] %s(): ", l.prefix, fileName, line, funcName)
	l.logger.SetPrefix(prefix)
	l.logger.Print(msg)
}
