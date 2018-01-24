package logrus_appinsights

import (
	"errors"
	"fmt"
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

func TestGetData(t *testing.T) {
	assert := assert.New(t)

	const defaultMessage = "entry_message"

	tests := []struct {
		data     map[string]interface{}
		expected string
	}{
		{
			map[string]interface{}{},
			`{"message":"entry_message"}`,
		},
		{
			map[string]interface{}{"message": "field_message"},
			`{"message":"field_message"}`,
		},
		{
			map[string]interface{}{
				"name":  "apple",
				"price": 105,
				"color": "red",
			},
			`{"color":"red","message":"entry_message","name":"apple","price":105}`,
		},
		{
			map[string]interface{}{
				"name":    "apple",
				"price":   105,
				"color":   "red",
				"message": "field_message",
			},
			`{"color":"red","message":"field_message","name":"apple","price":105}`,
		},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		hook := AppInsightsHook{}
		entry := &logrus.Entry{
			Message: defaultMessage,
			Data:    tt.data,
		}

		actual := hook.getData(entry)
		assert.Equal(tt.expected, *actual, target)
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

// func TestFire(t *testing.T) {
// 	hook, err := New("TestClient", ai.Config{
// 		InstrumentationKey: "",
// 	})
// 	if err != nil || hook == nil {
// 		t.Errorf(err.Error())
// 		return
// 	}
// 	logrus.AddHook(hook)

// 	logger := logrus.New()
// 	logger.Hooks.Add(hook)

// 	f := logrus.Fields {
// 		"message?": "fieldMessage",
// 		"tag":		"fieldTag",
// 		"value":	"fieldValue",
// 	}

// 	logger.WithFields(f).Error("I see dead people!")

// 	return
// }
