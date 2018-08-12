# InstanceLogger

A general way to report errors to a google logging (stackdriver) service.

## Setup

```bash
go get github.com/moorage/instancelogger
```

## Usage

```go
import (
	"sync"
	"google.golang.org/api/option"

	"github.com/moorage/instancelogger"
)

// save your own reference
myInstanceLogger := instancelogger.New(
  &projectID,
  optionalClientOption,
  optionalWaitGroup,
)

// OR use the singleton global
instancelogger.NewSingleton(
  &projectID,
  optionalClientOption,
  optionalWaitGroup,
)

// Once you know your topic name
err := myInstanceLogger.Init(errorsTopicName, optionalInstanceName, optionalProjectID)
if err != nil {
	instanceLogger.Fatal(err)
}

// Report an error
myInstanceLogger.Error(fmt.Error("oops"))

// Same but using the singleton
instancelogger.Singleton().Error(fmt.Error("oops"))

// os.Exit(1) on an error
myInstanceLogger.Fatal(fmt.Error("oops"))

// Clean up nicely when done (topic publisher uses a goroutine)
myInstanceLogger.Stop()


// If you have a wait group, you can wait until instanceLogger has completed all remaining publishing
optionalWaitGroup.Wait()
```
