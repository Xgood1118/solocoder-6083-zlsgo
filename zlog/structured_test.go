package zlog_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/sohaha/zlsgo"
	"github.com/sohaha/zlsgo/zlog"
)

func TestJSONOutput(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var buf bytes.Buffer
	log := zlog.New(&buf, "json_test ", zlog.BitDefault|zlog.BitJSON)

	log.Info("test message")

	output := buf.String()
	t.EqualTrue(strings.Contains(output, `"timestamp"`))
	t.EqualTrue(strings.Contains(output, `"level"`))
	t.EqualTrue(strings.Contains(output, `"message"`))
	t.EqualTrue(strings.Contains(output, `"caller"`))
	t.EqualTrue(strings.Contains(output, `"prefix"`))
	t.EqualTrue(strings.Contains(output, "test message"))
	t.EqualTrue(strings.Contains(output, "INFO"))

	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	t.NoError(err)
	t.Equal("json_test ", parsed["prefix"])
	t.Equal("test message", parsed["message"])
}

func TestJSONFields(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var buf bytes.Buffer
	log := zlog.New(&buf, "", zlog.BitJSON)

	log.InfoJ("user login", zlog.JSONFields{
		"user_id": 123,
		"action":  "login",
		"success": true,
	})

	output := buf.String()
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	t.NoError(err)
	t.Equal("user login", parsed["message"])
	t.Equal(float64(123), parsed["user_id"])
	t.Equal("login", parsed["action"])
	t.Equal(true, parsed["success"])
}

func TestHookInjection(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var buf bytes.Buffer
	log := zlog.New(&buf, "", zlog.BitJSON)
	log.AddHook(func(level int, msg string, fields zlog.JSONFields) zlog.JSONFields {
		fields["trace_id"] = "trace-123"
		fields["service"] = "test-service"
		return fields
	})

	log.Info("hook test")

	output := buf.String()
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	t.NoError(err)
	t.Equal("hook test", parsed["message"])
	t.Equal("trace-123", parsed["trace_id"])
	t.Equal("test-service", parsed["service"])
}

func TestHookModify(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var buf bytes.Buffer
	log := zlog.New(&buf, "", zlog.BitJSON)
	log.AddHook(func(level int, msg string, fields zlog.JSONFields) zlog.JSONFields {
		if level == zlog.LogWarn {
			fields["alert"] = true
		}
		return fields
	})

	log.Warn("warning message")

	output := buf.String()
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	t.NoError(err)
	t.Equal(true, parsed["alert"])
}

func TestMultipleHooks(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var buf bytes.Buffer
	log := zlog.New(&buf, "", zlog.BitJSON)

	log.AddHook(func(level int, msg string, fields zlog.JSONFields) zlog.JSONFields {
		fields["hook1"] = "value1"
		return fields
	})
	log.AddHook(func(level int, msg string, fields zlog.JSONFields) zlog.JSONFields {
		fields["hook2"] = "value2"
		if v, ok := fields["hook1"]; ok && v == "value1" {
			fields["hook_order"] = "correct"
		}
		return fields
	})

	log.Info("multi hook test")

	output := buf.String()
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	t.NoError(err)
	t.Equal("value1", parsed["hook1"])
	t.Equal("value2", parsed["hook2"])
	t.Equal("correct", parsed["hook_order"])
}

func TestJSONLevels(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	levels := []struct {
		fn   func(l *zlog.Logger, msg string)
		name string
	}{
		{func(l *zlog.Logger, msg string) { l.Debug(msg) }, "DEBUG"},
		{func(l *zlog.Logger, msg string) { l.Info(msg) }, "INFO"},
		{func(l *zlog.Logger, msg string) { l.Warn(msg) }, "WARN"},
		{func(l *zlog.Logger, msg string) { l.Error(msg) }, "ERROR"},
	}

	for _, lv := range levels {
		var buf bytes.Buffer
		log := zlog.New(&buf, "", zlog.BitJSON)
		log.SetLevel(zlog.LogDebug)

		lv.fn(log, "level test")

		output := buf.String()
		var parsed map[string]interface{}
		err := json.Unmarshal([]byte(output), &parsed)
		t.NoError(err)
		t.Equal(lv.name, parsed["level"], "level mismatch for "+lv.name)
	}
}

func TestJSONJMethods(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	methods := []struct {
		fn   func(l *zlog.Logger, msg string, fields ...zlog.JSONFields)
		name string
	}{
		{func(l *zlog.Logger, msg string, fields ...zlog.JSONFields) { l.DebugJ(msg, fields...) }, "DEBUG"},
		{func(l *zlog.Logger, msg string, fields ...zlog.JSONFields) { l.InfoJ(msg, fields...) }, "INFO"},
		{func(l *zlog.Logger, msg string, fields ...zlog.JSONFields) { l.WarnJ(msg, fields...) }, "WARN"},
		{func(l *zlog.Logger, msg string, fields ...zlog.JSONFields) { l.ErrorJ(msg, fields...) }, "ERROR"},
	}

	for _, m := range methods {
		var buf bytes.Buffer
		log := zlog.New(&buf, "", zlog.BitJSON)
		log.SetLevel(zlog.LogDebug)

		m.fn(log, "jmethod test", zlog.JSONFields{"key": m.name})

		output := buf.String()
		var parsed map[string]interface{}
		err := json.Unmarshal([]byte(output), &parsed)
		t.NoError(err)
		t.Equal(m.name, parsed["level"])
		t.Equal(m.name, parsed["key"])
	}
}

func TestNonJSONUnchanged(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var buf bytes.Buffer
	log := zlog.New(&buf, "prefix ", zlog.BitDefault)

	log.Info("normal message")

	output := buf.String()
	t.EqualFalse(strings.Contains(output, `"timestamp"`))
	t.EqualTrue(strings.Contains(output, "prefix"))
	t.EqualTrue(strings.Contains(output, "normal message"))
	t.EqualTrue(strings.Contains(output, "INFO"))
}

func TestJSONCaller(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var buf bytes.Buffer
	log := zlog.New(&buf, "", zlog.BitJSON)

	log.Info("caller test")

	output := buf.String()
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	t.NoError(err)

	caller, ok := parsed["caller"].(string)
	t.EqualTrue(ok)
	t.EqualTrue(strings.Contains(caller, "structured_test.go"))
	t.EqualTrue(strings.Contains(caller, ":"))
}

func TestJSONConcurrent(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var buf bytes.Buffer
	log := zlog.New(&buf, "", zlog.BitJSON)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			log.InfoJ("concurrent", zlog.JSONFields{"n": n})
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	t.Equal(50, len(lines))

	for _, line := range lines {
		var parsed map[string]interface{}
		err := json.Unmarshal([]byte(line), &parsed)
		t.NoError(err)
	}
}

func TestEnableJSON(tt *testing.T) {
	t := zlsgo.NewTest(tt)

	var buf bytes.Buffer
	log := zlog.New(&buf, "test ", zlog.BitDefault)

	log.Info("before json")
	t.EqualFalse(strings.Contains(buf.String(), `"timestamp"`))

	buf.Reset()
	log.EnableJSON()

	log.Info("after json")
	output := buf.String()
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	t.NoError(err)
	t.Equal("after json", parsed["message"])
}
