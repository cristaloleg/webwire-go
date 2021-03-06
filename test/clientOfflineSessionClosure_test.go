package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	webwire "github.com/qbeon/webwire-go"
	webwireClient "github.com/qbeon/webwire-go/client"
)

// TestClientOfflineSessionClosure tests offline session closure
func TestClientOfflineSessionClosure(t *testing.T) {
	sessionStorage := make(map[string]*webwire.Session)

	currentStep := 1
	var createdSession *webwire.Session

	// Initialize webwire server
	server := setupServer(
		t,
		&serverImpl{
			onRequest: func(
				_ context.Context,
				conn webwire.Connection,
				_ webwire.Message,
			) (webwire.Payload, error) {
				if currentStep == 2 {
					// Expect the session to be removed
					if conn.HasSession() {
						t.Errorf("Expected client to be anonymous")
					}
					return nil, nil
				}

				// Try to create a new session
				if err := conn.CreateSession(nil); err != nil {
					return nil, err
				}

				// Return the key of the newly created session
				return nil, nil
			},
		},
		webwire.ServerOptions{
			SessionManager: &callbackPoweredSessionManager{
				// Saves the session
				SessionCreated: func(conn webwire.Connection) error {
					sess := conn.Session()
					sessionStorage[sess.Key] = sess
					return nil
				},
				// Finds session by key
				SessionLookup: func(key string) (
					webwire.SessionLookupResult,
					error,
				) {
					// Expect the key of the created session to be looked up
					if key != createdSession.Key {
						err := fmt.Errorf(
							"Expected and looked up session keys differ: %s | %s",
							createdSession.Key,
							key,
						)
						t.Fatalf("Session lookup mismatch: %s", err)
						return webwire.SessionLookupResult{}, err
					}

					if session, exists := sessionStorage[key]; exists {
						// Session found
						return webwire.SessionLookupResult{
							Creation:   session.Creation,
							LastLookup: session.LastLookup,
							Info: webwire.SessionInfoToVarMap(
								session.Info,
							),
						}, nil
					}

					// Expect the session to be found
					t.Fatalf(
						"Expected session (%s) not found in: %v",
						createdSession.Key,
						sessionStorage,
					)
					// Session not found
					return webwire.SessionLookupResult{},
						webwire.SessNotFoundErr{}
				},
			},
		},
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

	/*****************************************************************\
		Step 1 - Create session and disconnect
	\*****************************************************************/

	// Create a new session
	if _, err := client.connection.Request(
		context.Background(),
		"login",
		webwire.NewPayload(webwire.EncodingBinary, []byte("auth")),
	); err != nil {
		t.Fatalf("Auth request failed: %s", err)
	}

	createdSession = client.connection.Session()

	// Disconnect client without closing the session
	client.connection.Close()

	// Ensure the session isn't lost
	if client.connection.Status() == webwireClient.Connected {
		t.Fatal("Client is expected to be disconnected")
	}
	if client.connection.Session().Key == "" {
		t.Fatal("Session lost after disconnection")
	}

	/*****************************************************************\
		Step 2 - Close session, reconnect and verify
	\*****************************************************************/
	currentStep = 2

	if err := client.connection.CloseSession(); err != nil {
		t.Fatalf("Offline session closure failed: %s", err)
	}

	// Ensure the session is removed locally
	if client.connection.Session() != nil {
		t.Fatal("Session not removed")
	}

	// Reconnect
	if err := client.connection.Connect(); err != nil {
		t.Fatalf("Couldn't reconnect: %s", err)
	}

	// Ensure the client is anonymous
	if _, err := client.connection.Request(
		context.Background(),
		"verify-restored",
		webwire.NewPayload(webwire.EncodingBinary, []byte("isrestored?")),
	); err != nil {
		t.Fatalf("Second request failed: %s", err)
	}
}
