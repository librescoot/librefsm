package librefsm

import (
	"log/slog"
	"time"
)

// Context is passed to all state handlers and provides access to FSM operations
type Context struct {
	FSM       *Machine
	Event     *Event  // Current event being processed (nil during entry/exit)
	FromState StateID // State we're transitioning from
	ToState   StateID // State we're transitioning to
	Data      any     // User-provided application data
	Logger    *slog.Logger
}

// CurrentState returns the current active state
func (c *Context) CurrentState() StateID {
	return c.FSM.CurrentState()
}

// IsInState checks if the given state is current or an ancestor of current
func (c *Context) IsInState(id StateID) bool {
	return c.FSM.IsInState(id)
}

// StartTimer starts a named timer that will inject an event when it fires.
// If a timer with the same name exists, it is reset.
func (c *Context) StartTimer(name string, duration time.Duration, event Event) {
	c.FSM.startTimerInternal(name, duration, event, TimerScopeState, c.FSM.currentState)
}

// StartTimerGlobal starts a timer that won't be auto-cancelled on state exit
func (c *Context) StartTimerGlobal(name string, duration time.Duration, event Event) {
	c.FSM.startTimerInternal(name, duration, event, TimerScopeGlobal, "")
}

// StopTimer stops a timer by name. No-op if timer doesn't exist.
func (c *Context) StopTimer(name string) {
	c.FSM.StopTimer(name)
}

// ResetTimer stops and restarts a timer with a new duration
func (c *Context) ResetTimer(name string, duration time.Duration) {
	c.FSM.resetTimer(name, duration)
}

// TimerActive checks if a timer is currently running
func (c *Context) TimerActive(name string) bool {
	return c.FSM.TimerActive(name)
}

// Send queues an event for asynchronous processing
func (c *Context) Send(event Event) {
	c.FSM.Send(event)
}
