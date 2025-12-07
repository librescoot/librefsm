package librefsm

import "log/slog"

// StateID is a unique identifier for a state
type StateID string

// EventID is a unique identifier for an event type
type EventID string

// StateType classifies the behavior of a state
type StateType int

const (
	// StateNormal is a regular state that waits for events
	StateNormal StateType = iota
	// StateCondition evaluates its condition immediately on entry and transitions
	StateCondition
	// StateJunction is like condition but can execute entry action first
	StateJunction
	// StateFinal is a terminal state - no transitions out
	StateFinal
)

// TimerScope defines when a timer is automatically cancelled
type TimerScope int

const (
	// TimerScopeGlobal - timer lives until explicitly stopped or FSM stops
	TimerScopeGlobal TimerScope = iota
	// TimerScopeState - timer auto-cancelled when exiting the state that started it
	TimerScopeState
)

// Logger is the default logger used when none is provided
var Logger = slog.Default()
