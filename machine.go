package librefsm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Machine is the runtime FSM instance
type Machine struct {
	definition   *Definition
	currentState StateID
	mu           sync.RWMutex

	events  chan Event
	timers  map[string]*timerEntry
	timerMu sync.Mutex

	data                any
	logger              *slog.Logger
	stateChangeCallback func(from, to StateID)

	ctx    context.Context
	cancel context.CancelFunc

	// Computed hierarchy info
	children map[StateID][]StateID // Parent -> children
	depth    map[StateID]int       // State -> depth in hierarchy

	// Active states in hierarchy (for parallel states, future use)
	activeStates map[StateID]StateID // Parent -> active child
}

// MachineOption is a functional option for configuring a Machine
type MachineOption func(*Machine)

// WithEventQueueSize sets the event queue buffer size
func WithEventQueueSize(size int) MachineOption {
	return func(m *Machine) {
		m.events = make(chan Event, size)
	}
}

// WithLogger sets the logger for the machine
func WithLogger(logger *slog.Logger) MachineOption {
	return func(m *Machine) {
		m.logger = logger
	}
}

// WithData sets the application data accessible via Context
func WithData(data any) MachineOption {
	return func(m *Machine) {
		m.data = data
	}
}

// WithStateChangeCallback sets a callback invoked after each state change
func WithStateChangeCallback(fn func(from, to StateID)) MachineOption {
	return func(m *Machine) {
		m.stateChangeCallback = fn
	}
}

// OnStateChange sets a callback invoked after each state change.
// Can be called after Build() but before Start().
func (m *Machine) OnStateChange(fn func(from, to StateID)) {
	m.stateChangeCallback = fn
}

// Start initializes the machine and begins the event loop
func (m *Machine) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.activeStates = make(map[StateID]StateID)

	// Enter initial state
	if err := m.enterState(m.definition.initial, nil, ""); err != nil {
		return fmt.Errorf("failed to enter initial state: %w", err)
	}

	// Start event loop
	go m.eventLoop()

	return nil
}

// Stop gracefully shuts down the machine
func (m *Machine) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	m.StopAllTimers()
	return nil
}

// Send queues an event for asynchronous processing
func (m *Machine) Send(event Event) {
	select {
	case m.events <- event:
	default:
		m.logger.Warn("event queue full, dropping event", "event", event.ID)
	}
}

// SendSync sends an event and waits for it to be processed
func (m *Machine) SendSync(event Event) error {
	done := make(chan error, 1)
	wrapper := Event{
		ID: event.ID,
		Payload: &syncEventPayload{
			original: event.Payload,
			done:     done,
		},
	}
	m.Send(wrapper)
	return <-done
}

type syncEventPayload struct {
	original any
	done     chan error
}

// CurrentState returns the current leaf state
func (m *Machine) CurrentState() StateID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentState
}

// SetState forces a direct state change, bypassing normal event-driven transitions.
// This is useful for hybrid migrations where legacy code needs to set state directly.
// It properly exits the current state and enters the new state, running callbacks.
func (m *Machine) SetState(newState StateID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.definition.states[newState]; !ok {
		return fmt.Errorf("unknown state: %s", newState)
	}

	if m.currentState == newState {
		return nil
	}

	fromState := m.currentState

	// Exit current state
	if err := m.exitState(m.currentState); err != nil {
		return fmt.Errorf("exit state %s: %w", m.currentState, err)
	}

	// Enter new state
	if err := m.enterState(newState, nil, fromState); err != nil {
		return fmt.Errorf("enter state %s: %w", newState, err)
	}

	// Notify callback
	if m.stateChangeCallback != nil {
		m.stateChangeCallback(fromState, m.currentState)
	}

	return nil
}

// IsInState checks if the given state is the current state or an ancestor
func (m *Machine) IsInState(id StateID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isInStateInternal(id)
}

func (m *Machine) isInStateInternal(id StateID) bool {
	current := m.currentState
	for current != "" {
		if current == id {
			return true
		}
		state := m.definition.states[current]
		if state == nil {
			break
		}
		current = state.Parent
	}
	return false
}

// eventLoop processes events from the queue
func (m *Machine) eventLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case event := <-m.events:
			var syncDone chan error
			payload := event.Payload

			// Handle sync events
			if sp, ok := payload.(*syncEventPayload); ok {
				syncDone = sp.done
				payload = sp.original
			}

			actualEvent := Event{ID: event.ID, Payload: payload}
			err := m.processEvent(actualEvent)

			if syncDone != nil {
				syncDone <- err
			}
		}
	}
}

// processEvent handles a single event
func (m *Machine) processEvent(event Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Debug("processing event", "event", event.ID, "state", m.currentState)

	// Find all matching transitions
	transitions := m.findAllTransitions(event)
	if len(transitions) == 0 {
		m.logger.Debug("no transition found", "event", event.ID, "state", m.currentState)
		return nil
	}

	// Try each transition until one's guard passes
	ctx := m.makeContext(&event)
	for _, transition := range transitions {
		// No guard means transition is always allowed
		if transition.Guard == nil {
			m.logger.Debug("executing transition (no guard)", "event", event.ID, "from", transition.From, "to", transition.To)
			return m.executeTransition(transition, &event)
		}

		// Check guard
		if transition.Guard(ctx) {
			m.logger.Debug("executing transition (guard passed)", "event", event.ID, "from", transition.From, "to", transition.To)
			return m.executeTransition(transition, &event)
		}

		m.logger.Debug("guard rejected transition", "event", event.ID, "from", transition.From, "to", transition.To)
	}

	// All guards failed
	m.logger.Debug("all guards rejected", "event", event.ID, "state", m.currentState)
	return nil
}

// findAllTransitions finds all matching transitions for the event
// Returns transitions in priority order: current state, then ancestors, then wildcards
func (m *Machine) findAllTransitions(event Event) []*Transition {
	var matches []*Transition

	// Check transitions from current state and ancestors
	current := m.currentState
	for current != "" {
		for i := range m.definition.transitions {
			t := &m.definition.transitions[i]
			if t.Event == event.ID && t.From == current {
				matches = append(matches, t)
			}
		}
		state := m.definition.states[current]
		if state == nil {
			break
		}
		current = state.Parent
	}

	// Check wildcard transitions
	for i := range m.definition.transitions {
		t := &m.definition.transitions[i]
		if t.Event == event.ID && t.From == WildcardState {
			matches = append(matches, t)
		}
	}

	return matches
}

// executeTransition performs the state transition
func (m *Machine) executeTransition(t *Transition, event *Event) error {
	fromState := m.currentState
	toState := t.To

	m.logger.Debug("executing transition", "from", fromState, "to", toState, "event", event.ID)

	// Find LCA (Least Common Ancestor)
	lca := m.findLCA(fromState, toState)

	// Exit states up to (but not including) LCA
	if err := m.exitToAncestor(fromState, lca); err != nil {
		return fmt.Errorf("exit failed: %w", err)
	}

	// Execute transition action
	if t.Action != nil {
		ctx := m.makeContext(event)
		ctx.FromState = fromState
		ctx.ToState = toState
		if err := t.Action(ctx); err != nil {
			return fmt.Errorf("transition action failed: %w", err)
		}
	}

	// Enter states from LCA down to target
	if err := m.enterFromAncestor(toState, lca, event, fromState); err != nil {
		return fmt.Errorf("enter failed: %w", err)
	}

	// Notify callback
	if m.stateChangeCallback != nil && fromState != m.currentState {
		m.stateChangeCallback(fromState, m.currentState)
	}

	return nil
}

// findLCA finds the least common ancestor of two states
func (m *Machine) findLCA(a, b StateID) StateID {
	if a == b {
		return a
	}

	// Get ancestors of a (including a)
	ancestorsA := make(map[StateID]bool)
	current := a
	for current != "" {
		ancestorsA[current] = true
		state := m.definition.states[current]
		if state == nil {
			break
		}
		current = state.Parent
	}
	ancestorsA[""] = true // Root is always an ancestor

	// Walk up from b until we find a common ancestor
	current = b
	for {
		if ancestorsA[current] {
			return current
		}
		state := m.definition.states[current]
		if state == nil {
			break
		}
		current = state.Parent
	}

	return "" // Root
}

// exitToAncestor exits states from current up to (but not including) ancestor
func (m *Machine) exitToAncestor(from StateID, ancestor StateID) error {
	current := from
	for current != "" && current != ancestor {
		if err := m.exitState(current); err != nil {
			return err
		}
		state := m.definition.states[current]
		if state == nil {
			break
		}
		current = state.Parent
	}
	return nil
}

// enterFromAncestor enters states from ancestor down to target
func (m *Machine) enterFromAncestor(target StateID, ancestor StateID, event *Event, fromState StateID) error {
	// Handle special case: target is the ancestor itself
	// This happens when transitioning to a parent state
	if target == ancestor {
		return m.enterState(target, event, fromState)
	}

	// Build path from ancestor to target
	path := m.pathFromAncestor(target, ancestor)

	// Enter each state in the path
	// For the first state, use the fromState parameter
	// For subsequent states, use the previous state in the path
	prevState := fromState
	for _, stateID := range path {
		if err := m.enterState(stateID, event, prevState); err != nil {
			return err
		}
		prevState = stateID
	}

	return nil
}

// pathFromAncestor returns the path from ancestor to target (excluding ancestor)
func (m *Machine) pathFromAncestor(target StateID, ancestor StateID) []StateID {
	var path []StateID
	current := target
	for current != "" && current != ancestor {
		path = append([]StateID{current}, path...)
		state := m.definition.states[current]
		if state == nil {
			break
		}
		current = state.Parent
	}
	return path
}

// enterState enters a state and handles conditions/default children
func (m *Machine) enterState(id StateID, event *Event, fromState StateID) error {
	state := m.definition.states[id]
	if state == nil {
		return fmt.Errorf("state %q not found", id)
	}

	m.logger.Debug("entering state", "state", id, "type", state.Type)
	m.currentState = id

	// Start declarative timeout timer
	if state.Timeout > 0 && state.TimeoutEvent != "" {
		timerName := fmt.Sprintf("_timeout_%s", id)
		m.startTimerInternalWithAction(timerName, state.Timeout, Event{ID: state.TimeoutEvent}, TimerScopeState, id, state.TimeoutAction)
	}

	// Execute entry action (for junction, this runs before condition)
	if state.OnEnter != nil {
		ctx := m.makeContext(event)
		ctx.FromState = fromState
		ctx.ToState = id
		if err := state.OnEnter(ctx); err != nil {
			return fmt.Errorf("entry action failed for %q: %w", id, err)
		}
	}

	// Handle condition/junction states
	if state.Type == StateCondition || state.Type == StateJunction {
		if state.Condition != nil {
			ctx := m.makeContext(event)
			nextState := state.Condition(ctx)
			if nextState != "" {
				// Exit this state and enter the next
				if err := m.exitState(id); err != nil {
					return err
				}
				return m.enterState(nextState, event, id)
			}
		}
	}

	// Auto-enter default child
	if state.DefaultChild != "" {
		return m.enterState(state.DefaultChild, event, id)
	}

	return nil
}

// exitState exits a state
func (m *Machine) exitState(id StateID) error {
	state := m.definition.states[id]
	if state == nil {
		return nil
	}

	m.logger.Debug("exiting state", "state", id)

	// Cancel state-scoped timers
	m.cleanupTimersForState(id)

	// Cancel declared timers
	for _, timerName := range state.DeclaredTimers {
		m.StopTimer(timerName)
	}

	// Cancel declarative timeout timer
	timerName := fmt.Sprintf("_timeout_%s", id)
	m.StopTimer(timerName)

	// Execute exit action
	if state.OnExit != nil {
		ctx := m.makeContext(nil)
		if err := state.OnExit(ctx); err != nil {
			return fmt.Errorf("exit action failed for %q: %w", id, err)
		}
	}

	return nil
}

// makeContext creates a context for callbacks
func (m *Machine) makeContext(event *Event) *Context {
	return &Context{
		FSM:    m,
		Event:  event,
		Data:   m.data,
		Logger: m.logger,
	}
}

// StateHistory returns recent state history (not yet implemented)
func (m *Machine) StateHistory() []StateID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return []StateID{m.currentState}
}
