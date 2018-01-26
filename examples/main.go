package main

import (
	"time"

	"github.com/jjcollinge/logrus-appinsights"
	log "github.com/sirupsen/logrus"
)

func init() {
	hook, err := logrus_appinsights.New("my_client", logrus_appinsights.Config{
		InstrumentationKey: "instrumentation_key",
		MaxBatchSize:       10,              // optional
		MaxBatchInterval:   time.Second * 5, // optional
	})
	if err != nil || hook == nil {
		panic(err)
	}

	// set custom levels
	hook.SetLevels([]log.Level{
		log.PanicLevel,
		log.ErrorLevel,
	})

	// ignore fields
	hook.AddIgnore("private")
	log.AddHook(hook)
}

func main() {

	f := log.Fields{
		"field1":  "field1_value",
		"field2":  "field2_value",
		"private": "private_value",
	}

	// Send log to Application Insights
	for {
		log.WithFields(f).Error("my message")
		time.Sleep(time.Second * 1)
	}
}
