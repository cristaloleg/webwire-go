package test

import (
	"context"
	"testing"
	"time"

	tmdwg "github.com/qbeon/tmdwg-go"
	webwire "github.com/qbeon/webwire-go"
	webwireClient "github.com/qbeon/webwire-go/client"
)

// TestClientInitiatedSessionDestruction tests
// client-initiated session destruction
func TestClientInitiatedSessionDestruction(t *testing.T) {
	sessionCreationCallbackCalled := tmdwg.NewTimedWaitGroup(1, 1*time.Second)
	sessionDestructionCallbackCalled := tmdwg.NewTimedWaitGroup(1, 1*time.Second)
	var createdSession *webwire.Session
	expectedCredentials := webwire.NewPayload(
		webwire.EncodingUtf8,
		[]byte("secret_credentials"),
	)
	placeholderMessage := webwire.NewPayload(
		webwire.EncodingUtf8,
		[]byte("nothinginteresting"),
	)
	currentStep := 1

	// Initialize webwire server
	server := setupServer(
		t,
		&serverImpl{
			onRequest: func(
				_ context.Context,
				conn webwire.Connection,
				msg webwire.Message,
			) (webwire.Payload, error) {
				// On step 2 - verify session creation and correctness
				if currentStep == 2 {
					sess := conn.Session()
					compareSessions(t, createdSession, sess)
					if string(msg.Payload().Data()) != sess.Key {
						t.Errorf(
							"Clients session key doesn't match: "+
								"client: '%s' | server: '%s'",
							string(msg.Payload().Data()),
							sess.Key,
						)
					}
					return nil, nil
				}

				// On step 4 - verify session destruction
				if currentStep == 4 {
					sess := conn.Session()
					if sess != nil {
						t.Errorf(
							"Expected the session to be destroyed, got: %v",
							sess,
						)
					}
					return nil, nil
				}

				// On step 1 - authenticate and create a new session
				if err := conn.CreateSession(nil); err != nil {
					return nil, err
				}

				// Return the key of the newly created session
				return webwire.NewPayload(
					webwire.EncodingBinary,
					[]byte(conn.SessionKey()),
				), nil
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
		callbackPoweredClientHooks{
			OnSessionCreated: func(_ *webwire.Session) {
				// Mark the client-side session creation callback as executed
				sessionCreationCallbackCalled.Progress(1)
			},
			OnSessionClosed: func() {
				// Ensure this callback is called during the
				if currentStep != 3 {
					t.Errorf(
						"Client-side session destruction callback "+
							"called at wrong step (%d)",
						currentStep,
					)
				}
				sessionDestructionCallbackCalled.Progress(1)
			},
		},
	)

	/*****************************************************************\
		Step 1 - Session Creation
	\*****************************************************************/
	if err := client.connection.Connect(); err != nil {
		t.Fatalf("Couldn't connect: %s", err)
	}

	// Send authentication request
	authReqReply, err := client.connection.Request(
		context.Background(),
		"login",
		expectedCredentials,
	)
	if err != nil {
		t.Fatalf("Authentication request failed: %s", err)
	}

	createdSession = client.connection.Session()

	// Verify reply
	if createdSession.Key != string(authReqReply.Data()) {
		t.Fatalf(
			"Unexpected session key: %s | %s",
			createdSession.Key,
			string(authReqReply.Data()),
		)
	}

	// Wait for the client-side session creation callback to be executed
	if err := sessionCreationCallbackCalled.Wait(); err != nil {
		t.Fatal("Session creation callback not called")
	}

	// Ensure the session was locally created
	currentSessionAfterCreation := client.connection.Session()
	if currentSessionAfterCreation.Key == "" {
		t.Fatalf(
			"Expected session on client-side, got none: %v",
			currentSessionAfterCreation,
		)
	}

	/*****************************************************************\
		Step 2 - Session Creation Verification
	\*****************************************************************/
	currentStep = 2

	// Send a test-request to verify the session creation on the server
	if _, err := client.connection.Request(
		context.Background(),
		"verify-session-created",
		webwire.NewPayload(
			webwire.EncodingBinary,
			[]byte(client.connection.Session().Key),
		),
	); err != nil {
		t.Fatalf("Session creation verification request failed: %s", err)
	}

	/*****************************************************************\
		Step 3 - Client-Side Session Destruction
	\*****************************************************************/
	currentStep = 3

	// Request session destruction
	if err := client.connection.CloseSession(); err != nil {
		t.Fatalf("Failed closing session on the client: %s", err)
	}

	// Wait for the client-side session destruction callback to be called
	if err := sessionDestructionCallbackCalled.Wait(); err != nil {
		t.Fatal("Session destruction callback not called")
	}

	/*****************************************************************\
		Step 4 - Destruction Verification
	\*****************************************************************/
	currentStep = 4

	// Ensure the session is destroyed locally as well
	currentSessionAfterDestruction := client.connection.Session()
	if currentSessionAfterDestruction != nil {
		t.Fatalf(
			"Expected session to be destroyed on the client as well, "+
				"but still got: %v",
			currentSessionAfterDestruction,
		)
	}

	// Send a test-request to verify the session was destroyed on the server
	if _, err := client.connection.Request(
		context.Background(),
		"test-request",
		placeholderMessage,
	); err != nil {
		t.Fatalf("Session destruction verification request failed: %s", err)
	}
}
