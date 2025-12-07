package librefsm

import "time"

// State defines a state in the machine
type State struct {
	ID           StateID
	Parent       StateID   // Empty for root states
	Type         StateType // Normal, Condition, Junction, Final
	DefaultChild StateID   // Auto-enter this child on entry

	OnEnter func(ctx *Context) error
	OnExit  func(ctx *Context) error

	// For condition/junction states: evaluated on entry to determine next state
	Condition func(ctx *Context) StateID

	// Declarative timeout: auto-started on entry, auto-cancelled on exit
	Timeout      time.Duration
	TimeoutEvent EventID

	// Declared timers (for auto-cleanup on state exit)
	DeclaredTimers []string
}

// StateOption is a functional option for configuring a State
type StateOption func(*State)

// WithParent sets the parent state for hierarchy
func WithParent(parent StateID) StateOption {
	return func(s *State) {
		s.Parent = parent
	}
}

// WithDefaultChild sets the default child state to auto-enter
func WithDefaultChild(child StateID) StateOption {
	return func(s *State) {
		s.DefaultChild = child
	}
}

// WithOnEnter sets the entry action for the state
func WithOnEnter(fn func(*Context) error) StateOption {
	return func(s *State) {
		s.OnEnter = fn
	}
}

// WithOnExit sets the exit action for the state
func WithOnExit(fn func(*Context) error) StateOption {
	return func(s *State) {
		s.OnExit = fn
	}
}

// WithTimeout sets a declarative timeout that auto-starts on entry
func WithTimeout(duration time.Duration, event EventID) StateOption {
	return func(s *State) {
		s.Timeout = duration
		s.TimeoutEvent = event
	}
}

// WithTimer declares a named timer for auto-cleanup on state exit
func WithTimer(name string) StateOption {
	return func(s *State) {
		s.DeclaredTimers = append(s.DeclaredTimers, name)
	}
}
