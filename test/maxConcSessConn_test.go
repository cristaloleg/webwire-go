package test

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/qbeon/webwire-go"

	wwr "github.com/qbeon/webwire-go"
	wwrClient "github.com/qbeon/webwire-go/client"
)

// TestMaxConcSessConn tests 4 maximum concurrent connections of a session
func TestMaxConcSessConn(t *testing.T) {
	sessionStorage := make(map[string]*wwr.Session)

	var sessionKey string
	sessionKeyLock := sync.RWMutex{}
	concurrentConns := uint(4)

	// Initialize server
	server := setupServer(
		t,
		&serverImpl{
			onClientConnected: func(conn wwr.Connection) {
				// Created the session for the first connecting client only
				sessionKeyLock.Lock()
				defer sessionKeyLock.Unlock()
				if len(sessionKey) < 1 {
					if err := conn.CreateSession(nil); err != nil {
						t.Errorf(
							"Unexpected error during session creation: %s",
							err,
						)
					}
					sessionKey = conn.SessionKey()
				}
			},
		},
		wwr.ServerOptions{
			MaxSessionConnections: concurrentConns,
			SessionManager: &callbackPoweredSessionManager{
				// Saves the session
				SessionCreated: func(conn wwr.Connection) error {
					sess := conn.Session()
					sessionStorage[sess.Key] = sess
					return nil
				},
				// Finds session by key
				SessionLookup: func(key string) (
					webwire.SessionLookupResult,
					error,
				) {
					if session, exists := sessionStorage[key]; exists {
						return webwire.SessionLookupResult{
							Creation:   session.Creation,
							LastLookup: session.LastLookup,
							Info: webwire.SessionInfoToVarMap(
								session.Info,
							),
						}, nil
					}
					// Session not found
					return webwire.SessionLookupResult{},
						webwire.SessNotFoundErr{}
				},
			},
		},
	)

	// Initialize client
	clients := make([]*callbackPoweredClient, concurrentConns)
	for i := uint(0); i < concurrentConns; i++ {
		client := newCallbackPoweredClient(
			server.Addr().String(),
			wwrClient.Options{
				DefaultRequestTimeout: 2 * time.Second,
			},
			callbackPoweredClientHooks{},
		)
		clients[i] = client

		if err := client.connection.Connect(); err != nil {
			t.Fatalf("Couldn't connect client: %s", err)
		}

		// Restore the session for all clients except the first one
		if i > 0 {
			sessionKeyLock.RLock()
			if err := client.connection.RestoreSession(
				[]byte(sessionKey),
			); err != nil {
				t.Fatalf(
					"Unexpected error during manual session restoration: %s",
					err,
				)
			}
			sessionKeyLock.RUnlock()
		}
	}

	// Ensure that the last superfluous client is rejected
	superfluousClient := newCallbackPoweredClient(
		server.Addr().String(),
		wwrClient.Options{
			DefaultRequestTimeout: 2 * time.Second,
		},
		callbackPoweredClientHooks{},
	)

	if err := superfluousClient.connection.Connect(); err != nil {
		t.Fatalf("Couldn't connect superfluous client: %s", err)
	}

	// Try to restore the session and expect this operation to fail
	// due to reached limit
	sessionKeyLock.RLock()
	sessRestErr := superfluousClient.connection.RestoreSession(
		[]byte(sessionKey),
	)
	_, isMaxReachedErr := sessRestErr.(wwr.MaxSessConnsReachedErr)
	if !isMaxReachedErr {
		t.Fatalf(
			"Expected a MaxSessConnsReached error during "+
				"manual session restoration, got: %s | %s",
			reflect.TypeOf(sessRestErr),
			sessRestErr,
		)
	}
	sessionKeyLock.RUnlock()
}
