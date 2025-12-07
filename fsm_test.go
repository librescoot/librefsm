package librefsm

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// Test states
const (
	stateInit    StateID = "init"
	stateA       StateID = "a"
	stateB       StateID = "b"
	stateC       StateID = "c"
	stateParent  StateID = "parent"
	stateChild1  StateID = "child1"
	stateChild2  StateID = "child2"
	stateCond    StateID = "condition"
	stateJunc    StateID = "junction"
	stateFinal   StateID = "final"
)

// Test events
const (
	evGo      EventID = "go"
	evBack    EventID = "back"
	evNext    EventID = "next"
	evTimeout EventID = "timeout"
	evDone    EventID = "done"
)

func TestBasicTransition(t *testing.T) {
	def := NewDefinition().
		State(stateA).
		State(stateB).
		Transition(stateA, evGo, stateB).
		Transition(stateB, evBack, stateA).
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	if m.CurrentState() != stateA {
		t.Errorf("expected state %s, got %s", stateA, m.CurrentState())
	}

	if err := m.SendSync(Event{ID: evGo}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	if m.CurrentState() != stateB {
		t.Errorf("expected state %s, got %s", stateB, m.CurrentState())
	}

	if err := m.SendSync(Event{ID: evBack}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	if m.CurrentState() != stateA {
		t.Errorf("expected state %s, got %s", stateA, m.CurrentState())
	}
}

func TestEntryExitActions(t *testing.T) {
	var entryCount, exitCount int32

	def := NewDefinition().
		State(stateA,
			WithOnEnter(func(c *Context) error {
				atomic.AddInt32(&entryCount, 1)
				return nil
			}),
			WithOnExit(func(c *Context) error {
				atomic.AddInt32(&exitCount, 1)
				return nil
			}),
		).
		State(stateB).
		Transition(stateA, evGo, stateB).
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	// Entry should have fired once
	if atomic.LoadInt32(&entryCount) != 1 {
		t.Errorf("expected entry count 1, got %d", entryCount)
	}

	if err := m.SendSync(Event{ID: evGo}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	// Exit should have fired
	if atomic.LoadInt32(&exitCount) != 1 {
		t.Errorf("expected exit count 1, got %d", exitCount)
	}
}

func TestGuard(t *testing.T) {
	var allowed bool

	def := NewDefinition().
		State(stateA).
		State(stateB).
		Transition(stateA, evGo, stateB,
			WithGuard(func(c *Context) bool {
				return allowed
			}),
		).
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	// Guard blocks transition
	allowed = false
	m.SendSync(Event{ID: evGo})
	if m.CurrentState() != stateA {
		t.Errorf("guard should have blocked transition")
	}

	// Guard allows transition
	allowed = true
	m.SendSync(Event{ID: evGo})
	if m.CurrentState() != stateB {
		t.Errorf("guard should have allowed transition")
	}
}

func TestTransitionAction(t *testing.T) {
	var actionData string

	def := NewDefinition().
		State(stateA).
		State(stateB).
		Transition(stateA, evGo, stateB,
			WithAction(func(c *Context) error {
				actionData = "executed"
				return nil
			}),
		).
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	m.SendSync(Event{ID: evGo})

	if actionData != "executed" {
		t.Errorf("action was not executed")
	}
}

func TestHierarchicalStates(t *testing.T) {
	var entries []StateID

	def := NewDefinition().
		State(stateParent,
			WithDefaultChild(stateChild1),
			WithOnEnter(func(c *Context) error {
				entries = append(entries, stateParent)
				return nil
			}),
		).
		State(stateChild1,
			WithParent(stateParent),
			WithOnEnter(func(c *Context) error {
				entries = append(entries, stateChild1)
				return nil
			}),
		).
		State(stateChild2,
			WithParent(stateParent),
			WithOnEnter(func(c *Context) error {
				entries = append(entries, stateChild2)
				return nil
			}),
		).
		State(stateB).
		Transition(stateChild1, evNext, stateChild2).
		Transition(stateParent, evGo, stateB). // Transition from parent applies to children
		Initial(stateParent)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	// Should have entered parent then child1
	if len(entries) != 2 || entries[0] != stateParent || entries[1] != stateChild1 {
		t.Errorf("expected [parent, child1], got %v", entries)
	}

	if m.CurrentState() != stateChild1 {
		t.Errorf("expected current state %s, got %s", stateChild1, m.CurrentState())
	}

	// IsInState should work for hierarchy
	if !m.IsInState(stateParent) {
		t.Error("should be in parent state")
	}
	if !m.IsInState(stateChild1) {
		t.Error("should be in child1 state")
	}

	// Transition within parent (child1 -> child2)
	entries = nil
	m.SendSync(Event{ID: evNext})

	if m.CurrentState() != stateChild2 {
		t.Errorf("expected current state %s, got %s", stateChild2, m.CurrentState())
	}

	// Parent entry should NOT have been called (LCA optimization)
	for _, e := range entries {
		if e == stateParent {
			t.Error("parent should not have been re-entered")
		}
	}
}

func TestConditionState(t *testing.T) {
	var goToB bool

	def := NewDefinition().
		State(stateA).
		ConditionState(stateCond, func(c *Context) StateID {
			if goToB {
				return stateB
			}
			return stateC
		}).
		State(stateB).
		State(stateC).
		Transition(stateA, evGo, stateCond).
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	// Condition routes to C
	goToB = false
	m.SendSync(Event{ID: evGo})
	if m.CurrentState() != stateC {
		t.Errorf("expected state %s, got %s", stateC, m.CurrentState())
	}
}

func TestJunctionState(t *testing.T) {
	var actionRan bool

	def := NewDefinition().
		State(stateA).
		JunctionState(stateJunc,
			func(c *Context) StateID { return stateB },
			WithOnEnter(func(c *Context) error {
				actionRan = true
				return nil
			}),
		).
		State(stateB).
		Transition(stateA, evGo, stateJunc).
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	m.SendSync(Event{ID: evGo})

	// Junction entry action should have run
	if !actionRan {
		t.Error("junction entry action should have run")
	}

	// Should be in final state
	if m.CurrentState() != stateB {
		t.Errorf("expected state %s, got %s", stateB, m.CurrentState())
	}
}

func TestDeclarativeTimeout(t *testing.T) {
	def := NewDefinition().
		State(stateA,
			WithTimeout(50*time.Millisecond, evTimeout),
		).
		State(stateB).
		Transition(stateA, evTimeout, stateB).
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	if m.CurrentState() != stateB {
		t.Errorf("expected state %s after timeout, got %s", stateB, m.CurrentState())
	}
}

func TestImperativeTimer(t *testing.T) {
	def := NewDefinition().
		State(stateA,
			WithOnEnter(func(c *Context) error {
				c.StartTimer("test", 50*time.Millisecond, Event{ID: evTimeout})
				return nil
			}),
		).
		State(stateB).
		Transition(stateA, evTimeout, stateB).
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	// Timer should be active
	if !m.TimerActive("test") {
		t.Error("timer should be active")
	}

	// Wait for timer
	time.Sleep(100 * time.Millisecond)

	if m.CurrentState() != stateB {
		t.Errorf("expected state %s after timer, got %s", stateB, m.CurrentState())
	}

	// Timer should be gone
	if m.TimerActive("test") {
		t.Error("timer should not be active after firing")
	}
}

func TestTimerCancelOnStateExit(t *testing.T) {
	def := NewDefinition().
		State(stateA,
			WithOnEnter(func(c *Context) error {
				c.StartTimer("test", 200*time.Millisecond, Event{ID: evTimeout})
				return nil
			}),
		).
		State(stateB).
		State(stateC).
		Transition(stateA, evGo, stateB).
		Transition(stateA, evTimeout, stateC). // Should never fire
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	// Exit state A before timer fires
	m.SendSync(Event{ID: evGo})

	if m.CurrentState() != stateB {
		t.Errorf("expected state %s, got %s", stateB, m.CurrentState())
	}

	// Wait past when timer would have fired
	time.Sleep(250 * time.Millisecond)

	// Should still be in B (timer was cancelled)
	if m.CurrentState() != stateB {
		t.Errorf("expected state %s (timer should be cancelled), got %s", stateB, m.CurrentState())
	}
}

func TestApplicationData(t *testing.T) {
	type AppData struct {
		Counter int
	}

	def := NewDefinition().
		State(stateA,
			WithOnEnter(func(c *Context) error {
				data := c.Data.(*AppData)
				data.Counter++
				return nil
			}),
		).
		Initial(stateA)

	appData := &AppData{}

	m, err := def.Build(WithData(appData))
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	if appData.Counter != 1 {
		t.Errorf("expected counter 1, got %d", appData.Counter)
	}
}

func TestStateChangeCallback(t *testing.T) {
	var changes [][2]StateID

	def := NewDefinition().
		State(stateA).
		State(stateB).
		State(stateC).
		Transition(stateA, evGo, stateB).
		Transition(stateB, evNext, stateC).
		Initial(stateA)

	m, err := def.Build(
		WithStateChangeCallback(func(from, to StateID) {
			changes = append(changes, [2]StateID{from, to})
		}),
	)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	m.SendSync(Event{ID: evGo})
	m.SendSync(Event{ID: evNext})

	if len(changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(changes))
	}

	if changes[0] != [2]StateID{stateA, stateB} {
		t.Errorf("unexpected first change: %v", changes[0])
	}

	if changes[1] != [2]StateID{stateB, stateC} {
		t.Errorf("unexpected second change: %v", changes[1])
	}
}

func TestWildcardTransition(t *testing.T) {
	def := NewDefinition().
		State(stateA).
		State(stateB).
		State(stateC).
		Transition(stateA, evGo, stateB).
		AnyStateTransition(evDone, stateC). // From any state
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	// Go to B first
	m.SendSync(Event{ID: evGo})
	if m.CurrentState() != stateB {
		t.Errorf("expected state %s, got %s", stateB, m.CurrentState())
	}

	// Wildcard transition from B to C
	m.SendSync(Event{ID: evDone})
	if m.CurrentState() != stateC {
		t.Errorf("expected state %s, got %s", stateC, m.CurrentState())
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		def     *Definition
		wantErr bool
	}{
		{
			name:    "no initial state",
			def:     NewDefinition().State(stateA),
			wantErr: true,
		},
		{
			name:    "undefined initial",
			def:     NewDefinition().State(stateA).Initial(stateB),
			wantErr: true,
		},
		{
			name:    "undefined parent",
			def:     NewDefinition().State(stateA, WithParent(stateB)).Initial(stateA),
			wantErr: true,
		},
		{
			name:    "undefined transition target",
			def:     NewDefinition().State(stateA).Transition(stateA, evGo, stateB).Initial(stateA),
			wantErr: true,
		},
		{
			name:    "condition without function",
			def:     NewDefinition().ConditionState(stateCond, nil).Initial(stateCond),
			wantErr: true,
		},
		{
			name: "valid definition",
			def: NewDefinition().
				State(stateA).
				State(stateB).
				Transition(stateA, evGo, stateB).
				Initial(stateA),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEventPayload(t *testing.T) {
	var receivedPayload string

	def := NewDefinition().
		State(stateA).
		State(stateB).
		Transition(stateA, evGo, stateB,
			WithAction(func(c *Context) error {
				if c.Event != nil && c.Event.Payload != nil {
					receivedPayload = c.Event.Payload.(string)
				}
				return nil
			}),
		).
		Initial(stateA)

	m, err := def.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer m.Stop()

	m.SendSync(Event{ID: evGo, Payload: "test-data"})

	if receivedPayload != "test-data" {
		t.Errorf("expected payload 'test-data', got %q", receivedPayload)
	}
}
