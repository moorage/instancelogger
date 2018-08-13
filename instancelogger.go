package instancelogger

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/logging"
	"google.golang.org/api/option"
)

var singleton *InstanceLogger

// InstanceLogger is a general way to report errors to a google pubsub service.
// Call New() and then Init().  Call Stop() when done.
type InstanceLogger struct {
	errorTopicName *string
	instanceName   *string
	projectID      *string
	ctx            context.Context
	cancelFunc     context.CancelFunc
	client         *logging.Client
	clientOption   option.ClientOption
	waitGroup      *sync.WaitGroup
	logger         *logging.Logger
}

// ErrorMessage represents a pubsub topic message for an error for use in json unmarshalling
type ErrorMessage struct {
	Error        string  `json:"error"`
	Trace        string  `json:"trace"`
	InstanceName *string `json:"instanceName"`
}

// NewSingleton calls New and sets instancelogger's singleton instance to this.  Convenient if you
// want one global instancelogger for the whole app.  Also returns it.
func NewSingleton(clientOption option.ClientOption, waitGroup *sync.WaitGroup) *InstanceLogger {
	singleton = New(clientOption, waitGroup)
	return singleton
}

// Singleton returns the global instancelogger.  You need to have called NewSingleton first
func Singleton() *InstanceLogger {
	return singleton
}

// New creats a InstanceLogger *without a topic yet*.  Be sure to call Init()
// if projectID is nil, attempts to find it from the instance metadata
func New(clientOption option.ClientOption, waitGroup *sync.WaitGroup) *InstanceLogger {
	return &InstanceLogger{
		clientOption: clientOption,
		waitGroup:    waitGroup,
	}
}

// Init actually starts publishing to a topic.  If this is not called, errors will only go to Stderr
// If instanceName and/or projectID are nil, will have tried to use the instance metadata
func (il *InstanceLogger) Init(errorTopicName string, optionalInstanceName *string, optionalProjectID *string) error {
	il.errorTopicName = &errorTopicName
	if optionalInstanceName != nil {
		il.instanceName = optionalInstanceName
	}
	if optionalProjectID != nil {
		il.projectID = optionalProjectID
	}

	c := metadata.NewClient(
		&http.Client{
			Transport: userAgentTransport{
				userAgent: "moorage/instancelogger",
				base:      http.DefaultTransport,
			},
			Timeout: 2 * time.Second,
		},
	)

	if optionalProjectID == nil {
		foundProjectID, perr := c.ProjectID()
		if perr != nil {
			if !perr.(net.Error).Timeout() { // if timeout, just ignore
				return perr
			}
		} else {
			il.projectID = &foundProjectID
		}
	}

	il.ctx, il.cancelFunc = context.WithCancel(context.Background())

	var client *logging.Client
	var err error
	if il.clientOption != nil {
		client, err = logging.NewClient(il.ctx, *il.projectID, il.clientOption)
	} else {
		client, err = logging.NewClient(il.ctx, *il.projectID)
	}
	if err != nil {
		return err
	}
	il.client = client
	il.logger = client.Logger(errorTopicName)

	if optionalInstanceName == nil {
		foundInstanceName, err := c.InstanceName()
		if err != nil {
			if !err.(net.Error).Timeout() { // if timeout, just ignore
				return err
			}
		} else {
			il.instanceName = &foundInstanceName
		}
	}

	return nil
}

// Error tries to report to pubsub, otherwise just prints to Stderr
func (il *InstanceLogger) Error(err error) {
	if il.waitGroup != nil {
		il.waitGroup.Add(1)
	}
	if il.logger == nil {
		log.Printf("[ERROR:LOGGING-NOT-INIT'ED] %+v\n", err)

		if il.waitGroup != nil {
			il.waitGroup.Done()
		}
		return
	}

	errorMsg := ErrorMessage{
		Error:        fmt.Sprintf("%v", err),
		Trace:        string(debug.Stack()),
		InstanceName: il.instanceName,
	}

	// Adds an entry to the log buffer.
	il.logger.Log(logging.Entry{Payload: errorMsg})
	log.Printf("[ERROR:REPORTED] %+v\n", errorMsg)

	if il.waitGroup != nil {
		il.waitGroup.Done()
	}
}

// Fatal calls Err and os.Exit(1)
func (il *InstanceLogger) Fatal(err error) {
	il.Error(err)
	il.Stop()
	os.Exit(1)
}

// Stop Stop()s the topic and calls the cancel function if available
func (il *InstanceLogger) Stop() {
	// Closes the client and flushes the buffer to Stackdriver
	if il.client != nil {
		il.client.Close()
		il.client = nil
	} else if il.cancelFunc != nil {
		il.cancelFunc()
		il.cancelFunc = nil
	}
}

// userAgentTransport sets the User-Agent header before calling base.
type userAgentTransport struct {
	userAgent string
	base      http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface.
func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req)
}
