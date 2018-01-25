package logrus_appinsights

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
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
	assert := assert.New(t)

	hook, err := New("test", Config{})
	assert.Error(err)
	assert.Nil(hook)
}

func TestNewWithAppInsightsConfig(t *testing.T) {
	assert := assert.New(t)

	hook, err := NewWithAppInsightsConfig("test", nil)
	assert.Error(err)
	assert.Nil(hook)
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

func TestBuildTrace(t *testing.T) {
	// You cannot introspect an trace to check for
	// consitency with entry or expected trace.
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

var isSuccessful bool
var globalTest *testing.T

func TestFire(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(handleFire))
	defer server.Close()

	hook, err := New("TestClient", Config{
		InstrumentationKey: "NotEmpty",
		EndpointUrl:        server.URL,
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

	logger.WithFields(f).Error("I see dead people!")

	// Wait enough time to allow the handler to run
	time.Sleep(time.Second * 2)

	assert.True(t, isSuccessful)
}

func handleFire(w http.ResponseWriter, r *http.Request) {
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
	trace.assertPath(globalTest, "data.baseData.properties.message", "I see dead people!")
	trace.assertPath(globalTest, "data.baseData.properties.source_level", "error")
	trace.assertPath(globalTest, "data.baseData.properties.value", "fieldValue")
	trace.assertPath(globalTest, "data.baseData.properties.tag", "fieldTag")
	isSuccessful = true
}

func TestHandleFire(t *testing.T) {
	payload := "{\"name\":\"Microsoft.ApplicationInsights.Message\",\"time\":\"2018-01-25T12:13:42Z\",\"iKey\":\"NotEmpty\",\"tags\":{\"ai.cloud.role\":\"TestClient\",\"ai.device.id\":\"RAZER-BLADE\",\"ai.device.machineName\":\"RAZER-BLADE\",\"ai.device.os\":\"windows\",\"ai.device.roleInstance\":\"RAZER-BLADE\",\"ai.internal.sdkVersion\":\"go:0.3.1-pre\"},\"data\":{\"baseType\":\"MessageData\",\"baseData\":{\"ver\":2,\"properties\":{\"message\":\"I see dead people!\",\"source_level\":\"error\",\"source_timestamp\":\"2018-01-25 12:13:42.4839613 +0000 GMT m=+0.007540300\",\"tag\":\"fieldTag\",\"value\":\"fieldValue\"},\"message\":\"I see dead people!\",\"severityLevel\":3}}}"
	var postBody bytes.Buffer
	gzipWriter := gzip.NewWriter(&postBody)
	if _, err := gzipWriter.Write([]byte(payload)); err != nil {
		gzipWriter.Close()
		t.Errorf(err.Error())
	}

	gzipWriter.Close()

	fmt.Printf("%+v", string(postBody.Bytes()))
	reader := bytes.NewReader(postBody.Bytes())
	req, err := http.NewRequest("POST", "", reader)
	if err != nil {
		t.Errorf(err.Error())
	}

	globalTest = t
	var res http.ResponseWriter
	handleFire(res, req)
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

func (msg jsonMessage) assertPath(t *testing.T, path string, value interface{}) {
	const tolerance = 0.0001
	v, err := msg.getPath(path)
	if err != nil {
		t.Error(err.Error())
		return
	}

	if num, ok := value.(int); ok {
		if vnum, ok := v.(float64); ok {
			if math.Abs(float64(num)-vnum) > tolerance {
				t.Errorf("Data was unexpected at %s. Got %g want %d", path, vnum, num)
			}
		} else if vnum, ok := v.(int); ok {
			if vnum != num {
				t.Errorf("Data was unexpected at %s. Got %d want %d", path, vnum, num)
			}
		} else {
			t.Errorf("Expected value at %s to be a number, but was %T", path, v)
		}
	} else if num, ok := value.(float64); ok {
		if vnum, ok := v.(float64); ok {
			if math.Abs(num-vnum) > tolerance {
				t.Errorf("Data was unexpected at %s. Got %g want %g", path, vnum, num)
			}
		} else if vnum, ok := v.(int); ok {
			if math.Abs(num-float64(vnum)) > tolerance {
				t.Errorf("Data was unexpected at %s. Got %d want %g", path, vnum, num)
			}
		} else {
			t.Errorf("Expected value at %s to be a number, but was %T", path, v)
		}
	} else if str, ok := value.(string); ok {
		if vstr, ok := v.(string); ok {
			if str != vstr {
				t.Errorf("Data was unexpected at %s. Got '%s' want '%s'", path, vstr, str)
			}
		} else {
			t.Errorf("Expected value at %s to be a string, but was %T", path, v)
		}
	} else if bl, ok := value.(bool); ok {
		if vbool, ok := v.(bool); ok {
			if bl != vbool {
				t.Errorf("Data was unexpected at %s. Got %t want %t", path, vbool, bl)
			}
		} else {
			t.Errorf("Expected value at %s to be a bool, but was %T", path, v)
		}
	} else {
		t.Errorf("Unsupported type: %#v", value)
	}
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
