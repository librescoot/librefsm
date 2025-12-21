package librefsm

import (
	"time"
)

// timerEntry tracks a running timer
type timerEntry struct {
	timer      *time.Timer
	event      Event
	scope      TimerScope
	ownerState StateID
	duration   time.Duration
	action     func(*Context) error // Optional callback to run before sending event
}

// startTimerInternal starts a named timer with scope tracking
func (m *Machine) startTimerInternal(name string, duration time.Duration, event Event, scope TimerScope, owner StateID) {
	m.startTimerInternalWithAction(name, duration, event, scope, owner, nil)
}

// startTimerInternalWithAction starts a named timer with an optional action callback
func (m *Machine) startTimerInternalWithAction(name string, duration time.Duration, event Event, scope TimerScope, owner StateID, action func(*Context) error) {
	m.timerMu.Lock()
	defer m.timerMu.Unlock()

	// Cancel existing timer with same name
	if existing, ok := m.timers[name]; ok {
		existing.timer.Stop()
		delete(m.timers, name)
	}

	// Create new timer
	t := time.AfterFunc(duration, func() {
		m.timerMu.Lock()
		// Check timer still exists (wasn't cancelled)
		entry, ok := m.timers[name]
		if ok {
			timerAction := entry.action
			timerDuration := entry.duration
			delete(m.timers, name)
			m.timerMu.Unlock()

			m.logger.Debug("timer fired", "name", name, "event", event.ID)

			// Run action callback before sending event
			if timerAction != nil {
				ctx := m.makeContext(nil)
				if err := timerAction(ctx); err != nil {
					// Action failed - restart timer for retry instead of sending event
					m.logger.Debug("timer action failed, restarting timer", "name", name, "error", err)
					m.startTimerInternalWithAction(name, timerDuration, event, scope, owner, timerAction)
					return
				}
			}

			m.Send(event)
		} else {
			m.timerMu.Unlock()
		}
	})

	m.timers[name] = &timerEntry{
		timer:      t,
		event:      event,
		scope:      scope,
		ownerState: owner,
		duration:   duration,
		action:     action,
	}

	m.logger.Debug("timer started", "name", name, "duration", duration, "event", event.ID)
}

// StartTimer starts a named timer (global scope by default from external calls)
func (m *Machine) StartTimer(name string, duration time.Duration, event Event) {
	m.startTimerInternal(name, duration, event, TimerScopeGlobal, "")
}

// StopTimer stops a timer by name
func (m *Machine) StopTimer(name string) {
	m.timerMu.Lock()
	defer m.timerMu.Unlock()

	if entry, ok := m.timers[name]; ok {
		entry.timer.Stop()
		delete(m.timers, name)
		m.logger.Debug("timer stopped", "name", name)
	}
}

// StopAllTimers stops all running timers
func (m *Machine) StopAllTimers() {
	m.timerMu.Lock()
	defer m.timerMu.Unlock()

	for name, entry := range m.timers {
		entry.timer.Stop()
		m.logger.Debug("timer stopped (cleanup)", "name", name)
	}
	m.timers = make(map[string]*timerEntry)
}

// TimerActive checks if a timer is running
func (m *Machine) TimerActive(name string) bool {
	m.timerMu.Lock()
	defer m.timerMu.Unlock()
	_, ok := m.timers[name]
	return ok
}

// resetTimer resets a timer to a new duration (preserving the event)
func (m *Machine) resetTimer(name string, duration time.Duration) {
	m.timerMu.Lock()
	entry, ok := m.timers[name]
	if !ok {
		m.timerMu.Unlock()
		return
	}
	event := entry.event
	scope := entry.scope
	owner := entry.ownerState
	entry.timer.Stop()
	delete(m.timers, name)
	m.timerMu.Unlock()

	m.startTimerInternal(name, duration, event, scope, owner)
}

// cleanupTimersForState cancels all state-scoped timers owned by the given state
func (m *Machine) cleanupTimersForState(stateID StateID) {
	m.timerMu.Lock()
	defer m.timerMu.Unlock()

	for name, entry := range m.timers {
		if entry.scope == TimerScopeState && entry.ownerState == stateID {
			entry.timer.Stop()
			delete(m.timers, name)
			m.logger.Debug("timer cleaned up (state exit)", "name", name, "state", stateID)
		}
	}
}
