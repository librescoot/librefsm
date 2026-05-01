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
	Timeout       time.Duration
	TimeoutEvent  EventID
	TimeoutAction func(*Context) error // Optional callback to run before sending timeout event
	TimeoutTarget StateID               // If set, auto-creates transition on timeout (with generated event)

	// Declared timers (for auto-cleanup on state exit)
	DeclaredTimers []string

	// Events that are silently dropped while this state (or any descendant)
	// is active. Use this for inputs that should be observed at the I/O
	// layer but must not drive the FSM in this state — e.g. hop-on mode,
	// where kickstand/brake/seatbox events publish to Redis but must not
	// transition or fire callbacks.
	BlockedEvents []EventID
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

// WithTimeout sets a declarative timeout that auto-starts on entry.
// An optional third argument specifies a callback to run before the timeout event is sent.
func WithTimeout(duration time.Duration, event EventID, action ...func(*Context) error) StateOption {
	return func(s *State) {
		s.Timeout = duration
		s.TimeoutEvent = event
		if len(action) > 0 {
			s.TimeoutAction = action[0]
		}
	}
}

// WithTimeoutTransition sets a declarative timeout that automatically transitions to the target state.
// The transition is auto-created during Build() with a generated internal event.
// An optional third argument specifies a callback to run before the timeout transition occurs.
func WithTimeoutTransition(duration time.Duration, target StateID, action ...func(*Context) error) StateOption {
	return func(s *State) {
		s.Timeout = duration
		s.TimeoutTarget = target
		// Generate internal event name from state ID and target
		s.TimeoutEvent = EventID("__timeout_" + string(s.ID) + "_to_" + string(target))
		if len(action) > 0 {
			s.TimeoutAction = action[0]
		}
	}
}

// WithTimer declares a named timer for auto-cleanup on state exit
func WithTimer(name string) StateOption {
	return func(s *State) {
		s.DeclaredTimers = append(s.DeclaredTimers, name)
	}
}

// WithBlockedEvents declares events that the machine drops silently while
// this state (or any descendant) is active. Blocked events are checked
// before transition lookup; wildcard and ancestor transitions are also
// pre-empted.
func WithBlockedEvents(events ...EventID) StateOption {
	return func(s *State) {
		s.BlockedEvents = append(s.BlockedEvents, events...)
	}
}
