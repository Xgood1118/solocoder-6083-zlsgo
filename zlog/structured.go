package zlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/sohaha/zlsgo/ztime"
	"github.com/sohaha/zlsgo/zutil"
)

const (
	BitJSON int = 1 << (iota + 10)
)

type JSONFields map[string]interface{}

type StructuredHook func(level int, msg string, fields JSONFields) JSONFields

func (log *Logger) EnableJSON() *Logger {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.flag |= BitJSON
	return log
}

func (log *Logger) DisableJSON() *Logger {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.flag &^= BitJSON
	return log
}

func (log *Logger) IsJSONEnabled() bool {
	log.mu.RLock()
	defer log.mu.RUnlock()
	return log.flag&BitJSON != 0
}

func (log *Logger) AddHook(hook StructuredHook) *Logger {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.structuredHooks = append(log.structuredHooks, hook)
	return log
}

func (log *Logger) outputJSON(level int, s string, isWrap bool, calldDepth int, prefixText ...string) error {
	if log.writeBefore != nil && len(s) > 0 {
		p := s
		if isWrap && len(p) > 0 && p[len(p)-1] == '\n' {
			p = p[:len(p)-1]
		}
		for i := range log.writeBefore {
			if log.writeBefore[i](level, p) {
				return nil
			}
		}
	}

	fields := make(JSONFields, 8)

	t := ztime.Time(log.flag&BitMicroSeconds != 0)
	fields["timestamp"] = t.Format(time.RFC3339Nano)

	if level >= 0 && level < len(Levels) {
		levelText := Levels[level]
		levelText = trimLevelBrackets(levelText)
		fields["level"] = levelText
	}

	if log.flag&(BitShortFile|BitLongFile) != 0 {
		file, line := log.fileLocation(calldDepth)
		if file != "" {
			if log.flag&BitShortFile != 0 {
				lastSlash := -1
				for i := len(file) - 1; i >= 0; i-- {
					if file[i] == '/' {
						lastSlash = i
						break
					}
				}
				if lastSlash >= 0 {
					file = file[lastSlash+1:]
				}
			}
			fields["caller"] = file + ":" + strconv.Itoa(line)
		}
	}

	if log.prefix != "" {
		fields["prefix"] = log.prefix
	}

	msg := s
	if isWrap && len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	if len(prefixText) > 0 {
		msg = prefixText[0] + msg
	}
	fields["message"] = msg

	for _, hook := range log.structuredHooks {
		fields = hook(level, msg, fields)
	}

	buf := zutil.GetBuff(256)
	defer zutil.PutBuff(buf)

	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(fields); err != nil {
		return err
	}

	out := log.outputWriter(level)
	_, err := out.Write(buf.Bytes())
	return err
}

func trimLevelBrackets(s string) string {
	s = trimSpace(s)
	if len(s) >= 2 && s[0] == '[' && s[len(s)-1] == ']' {
		return s[1 : len(s)-1]
	}
	return s
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func (log *Logger) InfoJ(msg string, fields ...JSONFields) {
	if log.level < LogInfo {
		return
	}
	log.logJSON(LogInfo, msg, fields...)
}

func (log *Logger) DebugJ(msg string, fields ...JSONFields) {
	if log.level < LogDebug {
		return
	}
	log.logJSON(LogDebug, msg, fields...)
}

func (log *Logger) WarnJ(msg string, fields ...JSONFields) {
	if log.level < LogWarn {
		return
	}
	log.logJSON(LogWarn, msg, fields...)
}

func (log *Logger) ErrorJ(msg string, fields ...JSONFields) {
	if log.level < LogError {
		return
	}
	log.logJSON(LogError, msg, fields...)
}

func (log *Logger) FatalJ(msg string, fields ...JSONFields) {
	if log.level < LogFatal {
		return
	}
	log.logJSON(LogFatal, msg, fields...)
	osExit(1)
}

func (log *Logger) PanicJ(msg string, fields ...JSONFields) {
	if log.level < LogPanic {
		return
	}
	log.logJSON(LogPanic, msg, fields...)
	panic(msg)
}

func (log *Logger) SuccessJ(msg string, fields ...JSONFields) {
	if log.level < LogSuccess {
		return
	}
	log.logJSON(LogSuccess, msg, fields...)
}

func (log *Logger) TipsJ(msg string, fields ...JSONFields) {
	if log.level < LogTips {
		return
	}
	log.logJSON(LogTips, msg, fields...)
}

func (log *Logger) logJSON(level int, msg string, extraFields ...JSONFields) {
	var mergedFields JSONFields
	if len(extraFields) > 0 {
		mergedFields = make(JSONFields, len(extraFields)*4)
		for _, f := range extraFields {
			for k, v := range f {
				mergedFields[k] = v
			}
		}
	}

	if log.writeBefore != nil {
		for i := range log.writeBefore {
			if log.writeBefore[i](level, msg) {
				return
			}
		}
	}

	fields := make(JSONFields, 8)

	t := ztime.Time(log.flag&BitMicroSeconds != 0)
	fields["timestamp"] = t.Format(time.RFC3339Nano)

	if level >= 0 && level < len(Levels) {
		levelText := Levels[level]
		levelText = trimLevelBrackets(levelText)
		fields["level"] = levelText
	}

	if log.flag&(BitShortFile|BitLongFile) != 0 {
		var ok bool
		_, file, line, ok := runtime.Caller(log.calldDepth)
		if !ok {
			file = "unknown-file"
			line = 0
		}
		if log.flag&BitShortFile != 0 {
			lastSlash := -1
			for i := len(file) - 1; i >= 0; i-- {
				if file[i] == '/' {
					lastSlash = i
					break
				}
			}
			if lastSlash >= 0 {
				file = file[lastSlash+1:]
			}
		}
		fields["caller"] = file + ":" + strconv.Itoa(line)
	}

	if log.prefix != "" {
		fields["prefix"] = log.prefix
	}

	fields["message"] = msg

	for k, v := range mergedFields {
		fields[k] = v
	}

	for _, hook := range log.structuredHooks {
		fields = hook(level, msg, fields)
	}

	buf := zutil.GetBuff(256)
	defer zutil.PutBuff(buf)

	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(fields); err != nil {
		return
	}

	out := log.outputWriter(level)
	_, _ = out.Write(buf.Bytes())
}

func InfoJ(msg string, fields ...JSONFields) {
	log.InfoJ(msg, fields...)
}

func DebugJ(msg string, fields ...JSONFields) {
	log.DebugJ(msg, fields...)
}

func WarnJ(msg string, fields ...JSONFields) {
	log.WarnJ(msg, fields...)
}

func ErrorJ(msg string, fields ...JSONFields) {
	log.ErrorJ(msg, fields...)
}

func FatalJ(msg string, fields ...JSONFields) {
	log.FatalJ(msg, fields...)
}

func PanicJ(msg string, fields ...JSONFields) {
	log.PanicJ(msg, fields...)
}

func SuccessJ(msg string, fields ...JSONFields) {
	log.SuccessJ(msg, fields...)
}

func TipsJ(msg string, fields ...JSONFields) {
	log.TipsJ(msg, fields...)
}

func AddHook(hook StructuredHook) *Logger {
	return log.AddHook(hook)
}

func EnableJSON() *Logger {
	return log.EnableJSON()
}

type jsonBuffPool struct {
	pool sync.Pool
}

var jsonBufPool = &jsonBuffPool{
	pool: sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	},
}

func (p *jsonBuffPool) Get() *bytes.Buffer {
	return p.pool.Get().(*bytes.Buffer)
}

func (p *jsonBuffPool) Put(b *bytes.Buffer) {
	b.Reset()
	p.pool.Put(b)
}

func jsonEscapeString(w io.Writer, s string) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			_, _ = w.Write([]byte(`\"`))
		case '\\':
			_, _ = w.Write([]byte(`\\`))
		case '\n':
			_, _ = w.Write([]byte(`\n`))
		case '\r':
			_, _ = w.Write([]byte(`\r`))
		case '\t':
			_, _ = w.Write([]byte(`\t`))
		default:
			if c < 0x20 {
				_, _ = fmt.Fprintf(w, `\u%04x`, c)
			} else {
				_ = writeByte(w, c)
			}
		}
	}
}
