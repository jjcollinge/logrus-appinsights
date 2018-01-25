# Application Insights Hook for Logrus <img src="http://i.imgur.com/hTeVwmJ.png" width="40" height="40" alt=":walrus:" class="emoji" title=":walrus:"/>

## Usage

```go
import (
    "fmt"
    "time"

    "github.com/jjcollinge/logrus-appinsights"
	"github.com/sirupsen/logrus"
)

func main() {
	hook, err := logrus_appinsights.New("my_client", logrus_appinsights.Config{
		InstrumentationKey: "my_instrumentation_key",
		MaxBatchSize:       10,              // optional
		MaxBatchInterval:   time.Second * 5, // optional
	})
	if err != nil || hook == nil {
		fmt.Errorf("%+v", err)
	}

	hook.SetLevels([]logrus.Level{
		logrus.PanicLevel,
		logrus.ErrorLevel,
		logrus.InfoLevel,
	})

	logger := logrus.New()
	logger.Hooks.Add(hook)

	f := logrus.Fields{
		"my_tag": "tag_value",
		"my_key": "key_value",
	}

	logger.WithFields(f).Error("my message")
}
```