# InstanceLogger

A general way to report errors to a google pubsub service.

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

instanceLogger, err := instancelogger.New(
  &projectID,
  optionalClientOption,
  optionalWaitGroup,
)

// Once you know your topic name
instanceLogger.Init(errorsTopicName, optionalInstanceName)

// Report an error
instanceLogger.Error(fmt.Error("oops"))

// os.Exit(1) on an error
instanceLogger.Fatal(fmt.Error("oops"))

// Clean up nicely when done (topic publisher uses a goroutine)
instanceLogger.Stop()


// If you have a wait group, you can wait until instanceLogger has completed all remaining publishing
optionalWaitGroup.Wait()
```
