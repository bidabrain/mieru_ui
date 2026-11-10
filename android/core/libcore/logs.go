package libcore

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/metacubex/mihomo/log"
)

type logLine struct {
	Seq   int64  `json:"seq"`
	Time  int64  `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

const logCap = 600

var (
	logMu  sync.Mutex
	logBuf []logLine
	logSeq int64
)

func appendLog(level, msg string) {
	logMu.Lock()
	defer logMu.Unlock()
	logSeq++
	logBuf = append(logBuf, logLine{Seq: logSeq, Time: time.Now().UnixMilli(), Level: level, Msg: msg})
	if len(logBuf) > logCap {
		logBuf = append([]logLine(nil), logBuf[len(logBuf)-logCap:]...)
	}
}

func collectLogs() {
	sub := log.Subscribe()
	for ev := range sub {
		appendLog(ev.Type(), ev.Payload)
	}
}

// logsAfter returns lines with seq > after.
func logsAfter(after int64) string {
	logMu.Lock()
	defer logMu.Unlock()
	out := struct {
		Next  int64     `json:"next"`
		Lines []logLine `json:"lines"`
	}{Next: logSeq, Lines: []logLine{}}
	for _, l := range logBuf {
		if l.Seq > after {
			out.Lines = append(out.Lines, l)
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func clearLogs() {
	logMu.Lock()
	logBuf = nil
	logMu.Unlock()
}
