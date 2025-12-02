package log

import (
	"fmt"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu  sync.RWMutex
	lg  *stdlog.Logger
	on  = true
	out io.Closer
)

const (
	cR  = "\033[0m"
	cD  = "\033[2m"
	cRd = "\033[31m"
	cGn = "\033[32m"
	cYl = "\033[33m"
	cBl = "\033[34m"
	cMg = "\033[35m"
	cCy = "\033[36m"
)

func Init(path string) {
	mu.Lock()
	defer mu.Unlock()

	a := path
	if !filepath.IsAbs(a) {
		e, _ := os.Executable()
		b := filepath.Dir(e)
		a = filepath.Join(b, path)
	}
	if err := os.MkdirAll(filepath.Dir(a), 0o755); err != nil {
		lg = stdlog.New(os.Stderr, "", 0)
		_ = lg.Output(2, "log init fallback stderr: "+err.Error())
		return
	}
	f, err := os.OpenFile(a, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		lg = stdlog.New(os.Stderr, "", 0)
		_ = lg.Output(2, "log open fallback stderr: "+err.Error())
		return
	}
	out = f
	lg = stdlog.New(io.MultiWriter(f, os.Stderr), "", 0)
	on = true
}

func Disable() {
	mu.Lock()
	defer mu.Unlock()
	on = false
}

func LogInfo(tag, msg string)  { fx("INFO", tag, msg) }
func LogDebug(tag, msg string) { fx("DEBUG", tag, msg) }
func LogError(tag, msg string) { fx("ERROR", tag, msg) }

func fx(level, tag, msg string) {
	mu.RLock()
	defer mu.RUnlock()
	if !on {
		return
	}
	if lg == nil {
		lg = stdlog.New(os.Stderr, "", 0)
	}

	ts := time.Now().Format(time.RFC3339)
	lc := cBl
	switch level {
	case "INFO":
		lc = cCy
	case "DEBUG":
		lc = cMg
	case "ERROR":
		lc = cRd
	}
	tc := cGn

	line := fmt.Sprintf("%s%s%s [%s%s%s] %s%s%s: %s", cD, ts, cR, lc, level, cR, tc, tag, cR, msg)
	_ = lg.Output(3, line)
}
