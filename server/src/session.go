// General purpose session functions

package main

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/Hanabi-Live/hanabi-live/logger"
	"github.com/gabstv/melody"
	"github.com/sasha-s/go-deadlock"
)

type sessionOutbound interface {
	Ready() bool
	Write([]byte) error
}

type melodySessionOutbound struct {
	session *melody.Session
}

func (outbound melodySessionOutbound) Ready() bool {
	return outbound.session != nil && outbound.session.Request != nil
}

func (outbound melodySessionOutbound) Write(message []byte) error {
	return outbound.session.Write(message)
}

type Session struct {
	// The underlying Melody session (which represents the WebSocket connection)
	ms *melody.Session

	// The checked outbound seam is overridden only by another concrete adapter,
	// such as an in-memory recorder. Normal websocket sessions use ms above.
	outbound sessionOutbound

	// Static data fields
	// (these do not change, so they can safely be read without race conditions or locking)
	SessionID uint64
	UserID    int
	Username  string
	Muted     bool // Users are forcefully disconnected upon being muted, so this is static
	FakeUser  bool

	// Dynamic data fields
	// (they are updated as the user performs activities, so we need to use a mutex)
	Data      *SessionData
	DataMutex *deadlock.RWMutex
}

type SessionData struct {
	Status             int
	TableID            uint64
	Friends            map[int]struct{}
	ReverseFriends     map[int]struct{}
	Hyphenated         bool
	Inactive           bool
	RateLimitAllowance float64
	RateLimitLastCheck time.Time
	Banned             bool
}

var (
	// The counter is atomically incremented before assignment,
	// so the first session ID will be 1 and will increase from there
	sessionIDCounter uint64 = 0
)

func NewSession() *Session {
	return &Session{
		ms: nil,

		SessionID: uint64(0),
		UserID:    0,
		Username:  "[unknown]",
		Muted:     false,
		FakeUser:  false,

		Data: &SessionData{
			Status:             StatusLobby, // By default, new users are in the lobby
			TableID:            uint64(0),   // 0 is used as a null value
			Friends:            make(map[int]struct{}),
			ReverseFriends:     make(map[int]struct{}),
			Hyphenated:         false,
			Inactive:           false,
			RateLimitAllowance: RateLimitRate,
			RateLimitLastCheck: time.Now(),
			Banned:             false,
		},
		DataMutex: &deadlock.RWMutex{},
	}
}

// NewFakeSession prepares a "fake" user session that will be used for game emulation
func NewFakeSession(id int, name string) *Session {
	s := NewSession()
	s.SessionID = uint64(id)
	s.UserID = id
	s.Username = name
	s.FakeUser = true

	return s
}

// Emit sends a message to a client using the Golem-style protocol described above
func (s *Session) Emit(command string, d interface{}) {
	_ = s.EmitChecked(command, d)
}

// EmitChecked sends one message and reports whether it reached the websocket
// adapter. Callers that own retryable work must use this method.
func (s *Session) EmitChecked(command string, d interface{}) error {
	if s == nil {
		return errors.New("websocket session is nil")
	}
	outbound := s.outbound
	if outbound == nil && s.ms != nil {
		outbound = melodySessionOutbound{session: s.ms}
	}
	if outbound == nil || !outbound.Ready() {
		return errors.New("websocket session is not ready")
	}

	data, err := json.Marshal(d)
	if err != nil {
		logger.Error("Failed to marshal data when writing to a WebSocket session: " + err.Error())
		return err
	}
	message := []byte(command + " " + string(data))
	return outbound.Write(message)
}

func (s *Session) Warning(message string) {
	logger.Info("Warning - " + message + " - " + s.Username)

	type WarningMessage struct {
		Warning string `json:"warning"`
	}
	s.Emit("warning", &WarningMessage{
		Warning: message,
	})
}

// Sent to the client if either their command was unsuccessful or something else went wrong
func (s *Session) Error(message string) {
	logger.Info("Error - " + message + " - " + s.Username)

	type ErrorMessage struct {
		Error string `json:"error"`
	}
	s.Emit("error", &ErrorMessage{
		Error: message,
	})
}
