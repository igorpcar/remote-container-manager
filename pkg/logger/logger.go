package logger

import (
	"fmt"
	"log"
	"time"
)

func init() {
	// disable standard log flags to format custom timestamp and level prefix
	log.SetFlags(0)
}

// color ANSI escape codes for terminal formatting
const (
	colorReset = "\033[0m"
	colorGreen = "\033[1;32m"
	colorCyan  = "\033[1;36m"
	colorRed   = "\033[1;31m"
)

// info logs general application events in green with [INF] prefix
func Info(format string, v ...interface{}) {
	prefix := colorGreen + "[INF]" + colorReset
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf(format, v...)
	log.Printf("%s %s %s", timestamp, prefix, msg)
}

// error logs error events in red with [ERR] prefix
func Error(format string, v ...interface{}) {
	prefix := colorRed + "[ERR]" + colorReset
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf(format, v...)
	log.Printf("%s %s %s", timestamp, prefix, msg)
}

// tracein logs incoming nats messages in cyan with [TRC] prefix and <<<<< indicator
func TraceIn(subject, reply string, data []byte) {
	prefix := colorCyan + "[TRC]" + colorReset
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	replyInfo := ""
	if reply != "" {
		replyInfo = fmt.Sprintf(" (reply: %s)", reply)
	}
	log.Printf("%s %s <<<<< RECV [%s]%s (%d bytes):\n  %s", timestamp, prefix, subject, replyInfo, len(data), string(data))
}

// traceout logs outgoing nats messages in cyan with [TRC] prefix and >>>>> indicator
func TraceOut(subject string, data []byte) {
	prefix := colorCyan + "[TRC]" + colorReset
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	log.Printf("%s %s >>>>> SEND [%s] (%d bytes):\n  %s", timestamp, prefix, subject, len(data), string(data))
}
