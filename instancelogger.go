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

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"

	"cloud.google.com/go/compute/metadata"
)

// InstanceLogger is a general way to report errors to a google pubsub service.
// Call New() and then Init().  Call Stop() when done.
type InstanceLogger struct {
	errorTopicName *string
	instanceName   *string
	projectID      *string
	ctx            context.Context
	cancelFunc     context.CancelFunc
	client         *pubsub.Client
	clientOption   option.ClientOption
	waitGroup      *sync.WaitGroup
	topic          *pubsub.Topic
}

// ErrorTopicMessage represents a pubsub topic message for an error for use in json unmarshalling
type ErrorTopicMessage struct {
	Error        string  `json:"error"`
	Trace        string  `json:"trace"`
	InstanceName *string `json:"instanceName"`
}

// New creats a InstanceLogger *without a topic yet*.  Be sure to call Init()
// if projectID is nil, attempts to find it from the instance metadata
func New(optionalProjectID *string, clientOption option.ClientOption, waitGroup *sync.WaitGroup) (*InstanceLogger, error) {
	il := &InstanceLogger{
		projectID:    optionalProjectID,
		clientOption: clientOption,
		waitGroup:    waitGroup,
	}
	il.ctx, il.cancelFunc = context.WithCancel(context.Background())
	var client *pubsub.Client
	var err error
	if clientOption != nil {
		client, err = pubsub.NewClient(il.ctx, *optionalProjectID, clientOption)
	} else {
		client, err = pubsub.NewClient(il.ctx, *optionalProjectID)
	}

	if err != nil {
		return nil, err
	}

	il.client = client

	c := metadata.NewClient(
		&http.Client{
			Transport: userAgentTransport{
				userAgent: "moorage/instancelogger",
				base:      http.DefaultTransport,
			},
		},
	)
	if optionalProjectID == nil {
		foundProjectID, perr := c.ProjectID()
		if perr != nil {
			if !perr.(net.Error).Timeout() { // if timeout, just ignore
				return nil, perr
			}
		} else {
			il.projectID = &foundProjectID
		}
	}
	foundInstanceName, err := c.InstanceName()
	if err != nil {
		if !err.(net.Error).Timeout() { // if timeout, just ignore
			return nil, err
		}
	} else {
		il.instanceName = &foundInstanceName
	}

	return il, nil
}

// Init actually starts publishing to a topic.  If this is not set, errors will only go to Stderr
// If instanceName is nil, will have tried to use the instance metadata
func (il *InstanceLogger) Init(errorTopicName string, instanceName *string) {
	il.errorTopicName = &errorTopicName
	if instanceName != nil {
		il.instanceName = instanceName
	}
	il.topic = il.client.Topic(errorTopicName)
}

// Error tries to report to pubsub, otherwise just prints to Stderr
func (il *InstanceLogger) Error(err error) {
	if il.waitGroup != nil {
		il.waitGroup.Add(1)
	}
	if il.topic == nil {
		log.Printf("[ERROR] %+v\n", err)

		if il.waitGroup != nil {
			il.waitGroup.Done()
		}
		return
	}

	var errorMsg string
	if il.instanceName != nil {
		errorMsg = fmt.Sprintf(
			"{\"error\": \"%v\", \"trace\":\"%v\",\"instanceName\":\"%s\"}",
			err,
			string(debug.Stack()),
			*il.instanceName,
		)
	} else {
		errorMsg = fmt.Sprintf(
			"{\"error\": \"%v\", \"trace\":\"%v\",\"instanceName\": null}",
			err,
			string(debug.Stack()),
		)
	}

	result := il.topic.Publish(il.ctx, &pubsub.Message{
		Data: []byte(errorMsg),
	})
	id, err := result.Get(il.ctx)
	if err != nil {
		log.Printf("[ERROR] %+v\n", err)
	} else {
		log.Printf("[REPORTED_ERR(%s)] %s\n", id, errorMsg)
	}

	if il.waitGroup != nil {
		il.waitGroup.Done()
	}
}

// Fatal calls Err and os.Exit(1)
func (il *InstanceLogger) Fatal(err error) {
	il.Error(err)
	os.Exit(1)
}

// Stop Stop()s the topic and calls the cancel function if available
func (il *InstanceLogger) Stop() {
	if il.topic != nil {
		il.topic.Stop()
	}
	if il.cancelFunc != nil {
		il.cancelFunc()
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
