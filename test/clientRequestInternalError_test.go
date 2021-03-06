package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	webwire "github.com/qbeon/webwire-go"
	webwireClient "github.com/qbeon/webwire-go/client"
)

// TestClientRequestInternalError tests returning of non-ReqErr errors
// from the request handler
func TestClientRequestInternalError(t *testing.T) {
	// Initialize webwire server given only the request
	server := setupServer(
		t,
		&serverImpl{
			onRequest: func(
				_ context.Context,
				_ webwire.Connection,
				_ webwire.Message,
			) (webwire.Payload, error) {
				// Fail the request by returning a non-ReqErr error
				return nil, fmt.Errorf(
					"don't worry, this internal error is expected",
				)
			},
		},
		webwire.ServerOptions{},
	)

	// Initialize client
	client := newCallbackPoweredClient(
		server.Addr().String(),
		webwireClient.Options{
			DefaultRequestTimeout: 2 * time.Second,
		},
		callbackPoweredClientHooks{},
	)

	if err := client.connection.Connect(); err != nil {
		t.Fatalf("Couldn't connect: %s", err)
	}

	// Send request and await reply
	reply, reqErr := client.connection.Request(
		context.Background(),
		"",
		webwire.NewPayload(webwire.EncodingUtf8, []byte("dummydata")),
	)

	// Verify returned error
	if reqErr == nil {
		t.Fatal("Expected an error, got nil")
	}

	if _, isInternalErr := reqErr.(webwire.ReqInternalErr); !isInternalErr {
		t.Fatalf("Expected an internal server error, got: %v", reqErr)
	}

	if reply != nil {
		t.Fatalf(
			"Reply should have been empty, but was: '%s' (%d)",
			string(reply.Data()),
			len(reply.Data()),
		)
	}
}
