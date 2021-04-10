package app_insights


import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	hook, err := New("test")
	assert.NotNil(t, hook)
	assert.Nil(t, err)
}

func TestNewWithAppInsightsConfig(t *testing.T) {
	hook, err := New("asdfads")
	assert.NotNil(t, hook)
	assert.Nil(t, err)
}

func TestLevels(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		levels []logrus.Level
	}{
		{nil},
		{[]logrus.Level{logrus.WarnLevel}},
		{[]logrus.Level{logrus.ErrorLevel}},
		{[]logrus.Level{logrus.WarnLevel, logrus.DebugLevel}},
		{[]logrus.Level{logrus.WarnLevel, logrus.DebugLevel, logrus.ErrorLevel}},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		hook := AppInsightsHook{}
		levels := hook.Levels()
		assert.Nil(levels, target)

		hook.levels = tt.levels
		levels = hook.Levels()
		assert.Equal(tt.levels, levels, target)
	}
}

func TestSetLevels(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		levels []logrus.Level
	}{
		{nil},
		{[]logrus.Level{logrus.WarnLevel}},
		{[]logrus.Level{logrus.ErrorLevel}},
		{[]logrus.Level{logrus.WarnLevel, logrus.DebugLevel}},
		{[]logrus.Level{logrus.WarnLevel, logrus.DebugLevel, logrus.ErrorLevel}},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		hook := AppInsightsHook{}
		assert.Nil(hook.levels, target)

		hook.SetLevels(tt.levels)
		assert.Equal(tt.levels, hook.levels, target)

		hook.SetLevels(nil)
		assert.Nil(hook.levels, target)
	}
}

func TestAddIgnore(t *testing.T) {
	assert := assert.New(t)

	hook := AppInsightsHook{
		ignoreFields: make(map[string]struct{}),
	}

	list := []string{"foo", "bar", "baz"}
	for i, key := range list {
		assert.Len(hook.ignoreFields, i)

		hook.AddIgnore(key)
		assert.Len(hook.ignoreFields, i+1)

		for j := 0; j <= i; j++ {
			assert.Contains(hook.ignoreFields, list[j])
		}
	}
}

func TestAddFilter(t *testing.T) {
	assert := assert.New(t)

	hook := AppInsightsHook{
		filters: make(map[string]func(interface{}) interface{}),
	}

	list := []string{"foo", "bar", "baz"}
	for i, key := range list {
		assert.Len(hook.filters, i)

		hook.AddFilter(key, nil)
		assert.Len(hook.filters, i+1)

		for j := 0; j <= i; j++ {
			assert.Contains(hook.filters, list[j])
		}
	}
}

func TestFormatData(t *testing.T) {
	assert := assert.New(t)

	// assertion types
	var (
		assertTypeInt    int
		assertTypeString string
		assertTypeTime   time.Time
	)

	tests := []struct {
		name         string
		value        interface{}
		expectedType interface{}
	}{
		{"int", 13, assertTypeInt},
		{"string", "foo", assertTypeString},
		{"error", errors.New("this is a test error"), assertTypeString},
		{"time_stamp", time.Now(), assertTypeTime},        // implements JSON marshaler
		{"time_duration", time.Hour, assertTypeString},    // implements .String()
		{"stringer", myStringer{}, assertTypeString},      // implements .String()
		{"stringer_ptr", &myStringer{}, assertTypeString}, // implements .String()
		{"not_stringer", notStringer{}, notStringer{}},
		{"not_stringer_ptr", &notStringer{}, &notStringer{}},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		result := formatData(tt.value)
		assert.IsType(tt.expectedType, result, target)
	}
}

type myStringer struct{}

func (myStringer) String() string { return "myStringer!" }

type notStringer struct{}

func (notStringer) String() {}

func TestStringPtr(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		value string
	}{
		{"abc"},
		{""},
		{"991029102910291029478748"},
		{"skjdklsajdlewrjo4iuoivjcklxmc,.mklrjtlkrejijoijpoijvpodjfr"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		p := stringPtr(tt.value)
		assert.Equal(tt.value, *p, target)
	}
}

type RequestContext struct {
	statusCode int
	server     *httptest.Server
	doneChan   chan bool
}

func TestFire(t *testing.T) {
	assert := assert.New(t)
	context := RequestContext{
		statusCode: 500,
	}
	context.doneChan = make(chan bool)
	context.server = httptest.NewServer(http.HandlerFunc(context.receiveHandler))
	defer context.server.Close()

	hook, err := NewWithAppInsightsConfig( &appinsights.TelemetryConfiguration{
		InstrumentationKey: "NotEmpty",
		EndpointUrl:        context.server.URL,
		MaxBatchSize:       1,
		MaxBatchInterval:   time.Millisecond * 10,
	})
	if err != nil || hook == nil {
		t.Errorf(err.Error())
	}
	logrus.AddHook(hook)

	logger := logrus.New()
	logger.Hooks.Add(hook)

	f := logrus.Fields{
		"tag":   "fieldTag",
		"value": "fieldValue",
	}

	// This should call our context server and receive handler.
	// We're not using the response writer to determine success,
	// we are checking the context object instead.
	logger.WithFields(f).Error("I see dead people!")

	_ = <-context.doneChan
	assert.Equal(context.statusCode, http.StatusOK, fmt.Sprintf("actual value %d did not match expected %d", context.statusCode, http.StatusOK))
}

func (c *RequestContext) receiveHandler(w http.ResponseWriter, r *http.Request) {
	reader, err := gzip.NewReader(r.Body)
	if err != nil {
		return
	}
	buffer := new(bytes.Buffer)
	buffer.ReadFrom(reader)
	j, err := parsePayload(buffer.Bytes())
	if err != nil {
		return
	}
	trace := j[0]
	testCases := map[string]string{
		"data.baseData.properties.message":      "I see dead people!",
		"data.baseData.properties.source_level": "error",
		"data.baseData.properties.value":        "fieldValue",
		"data.baseData.properties.tag":          "fieldTag",
	}
	for k, v := range testCases {
		if err := trace.assertPath(k, v); err != nil {
			c.statusCode = http.StatusBadRequest
			c.doneChan <- true
			return
		}
	}
	c.statusCode = http.StatusOK
	c.doneChan <- true
	return
}

func TestHandler(t *testing.T) {
	assert := assert.New(t)
	payload := "{\"name\":\"Microsoft.ApplicationInsights.Message\",\"time\":\"2018-01-25T12:13:42Z\",\"iKey\":\"NotEmpty\",\"tags\":{\"app_insights.cloud.role\":\"TestClient\",\"app_insights.device.id\":\"RAZER-BLADE\",\"app_insights.device.machineName\":\"RAZER-BLADE\",\"app_insights.device.os\":\"windows\",\"app_insights.device.roleInstance\":\"RAZER-BLADE\",\"app_insights.internal.sdkVersion\":\"go:0.3.1-pre\"},\"data\":{\"baseType\":\"MessageData\",\"baseData\":{\"ver\":2,\"properties\":{\"message\":\"I see dead people!\",\"source_level\":\"error\",\"source_timestamp\":\"2018-01-25 12:13:42.4839613 +0000 GMT m=+0.007540300\",\"tag\":\"fieldTag\",\"value\":\"fieldValue\"},\"message\":\"I see dead people!\",\"severityLevel\":3}}}"
	var postBody bytes.Buffer
	gzipWriter := gzip.NewWriter(&postBody)
	if _, err := gzipWriter.Write([]byte(payload)); err != nil {
		gzipWriter.Close()
		t.Errorf(err.Error())
	}

	gzipWriter.Close()

	reader := bytes.NewReader(postBody.Bytes())
	req, err := http.NewRequest("POST", "", reader)
	if err != nil {
		t.Errorf(err.Error())
	}

	context := RequestContext{
		statusCode: 500,
	}
	context.doneChan = make(chan bool)

	// We're not using the response writer to determine success,
	// we are checking the context object instead.
	var rw mockResponseWriter
	rw.HeaderMap = make(map[string][]string)
	go context.receiveHandler(&rw, req)

	_ = <-context.doneChan
	assert.Equal(context.statusCode, http.StatusOK, fmt.Sprintf("actual value %d did not match expected %d", context.statusCode, http.StatusOK))
}

type mockResponseWriter struct {
	HeaderMap http.Header
}

func (rw *mockResponseWriter) Header() http.Header {
	return rw.HeaderMap
}
func (rw *mockResponseWriter) WriteHeader(statusCode int) {
	rw.HeaderMap["status_code"][0] = strconv.Itoa(statusCode)
}
func (rw *mockResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}

type jsonMessage map[string]interface{}
type jsonPayload []jsonMessage

func parsePayload(payload []byte) (jsonPayload, error) {
	// json.Decoder can detect line endings for us but I'd like to explicitly find them.
	var result jsonPayload
	for _, item := range bytes.Split(payload, []byte("\n")) {
		if len(item) == 0 {
			continue
		}

		decoder := json.NewDecoder(bytes.NewReader(item))
		msg := make(jsonMessage)
		if err := decoder.Decode(&msg); err == nil {
			result = append(result, msg)
		} else {
			return result, err
		}
	}

	return result, nil
}

func (msg jsonMessage) assertPath(path string, value interface{}) error {
	const tolerance = 0.0001
	v, err := msg.getPath(path)
	if err != nil {
		return err
	}

	if num, ok := value.(int); ok {
		if vnum, ok := v.(float64); ok {
			if math.Abs(float64(num)-vnum) > tolerance {
				return fmt.Errorf("Data was unexpected at %s. Got %g want %d", path, vnum, num)
			}
		} else if vnum, ok := v.(int); ok {
			if vnum != num {
				return fmt.Errorf("Data was unexpected at %s. Got %d want %d", path, vnum, num)
			}
		} else {
			return fmt.Errorf("Expected value at %s to be a number, but was %T", path, v)
		}
	} else if num, ok := value.(float64); ok {
		if vnum, ok := v.(float64); ok {
			if math.Abs(num-vnum) > tolerance {
				return fmt.Errorf("Data was unexpected at %s. Got %g want %g", path, vnum, num)
			}
		} else if vnum, ok := v.(int); ok {
			if math.Abs(num-float64(vnum)) > tolerance {
				return fmt.Errorf("Data was unexpected at %s. Got %d want %g", path, vnum, num)
			}
		} else {
			return fmt.Errorf("Expected value at %s to be a number, but was %T", path, v)
		}
	} else if str, ok := value.(string); ok {
		if vstr, ok := v.(string); ok {
			if str != vstr {
				return fmt.Errorf("Data was unexpected at %s. Got '%s' want '%s'", path, vstr, str)
			}
		} else {
			return fmt.Errorf("Expected value at %s to be a string, but was %T", path, v)
		}
	} else if bl, ok := value.(bool); ok {
		if vbool, ok := v.(bool); ok {
			if bl != vbool {
				return fmt.Errorf("Data was unexpected at %s. Got %t want %t", path, vbool, bl)
			}
		} else {
			return fmt.Errorf("Expected value at %s to be a bool, but was %T", path, v)
		}
	} else {
		return fmt.Errorf("Unsupported type: %#v", value)
	}
	return nil
}

func (msg jsonMessage) getPath(path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	var obj interface{} = msg
	for i, part := range parts {
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			// Array
			idxstr := part[1 : len(part)-2]
			idx, _ := strconv.Atoi(idxstr)

			if ar, ok := obj.([]interface{}); ok {
				if idx >= len(ar) {
					return nil, fmt.Errorf("Index out of bounds: %s", strings.Join(parts[0:i+1], "."))
				}

				obj = ar[idx]
			} else {
				return nil, fmt.Errorf("Path %s is not an array", strings.Join(parts[0:i], "."))
			}
		} else if part == "<len>" {
			if ar, ok := obj.([]interface{}); ok {
				return len(ar), nil
			}
		} else {
			// Map
			if dict, ok := obj.(jsonMessage); ok {
				if val, ok := dict[part]; ok {
					obj = val
				} else {
					return nil, fmt.Errorf("Key %s not found in %s", part, strings.Join(parts[0:i], "."))
				}
			} else if dict, ok := obj.(map[string]interface{}); ok {
				if val, ok := dict[part]; ok {
					obj = val
				} else {
					return nil, fmt.Errorf("Key %s not found in %s", part, strings.Join(parts[0:i], "."))
				}
			} else {
				return nil, fmt.Errorf("Path %s is not a map", strings.Join(parts[0:i], "."))
			}
		}
	}

	return obj, nil
}
