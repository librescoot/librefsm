package librefsm

import (
	"fmt"
)

// Definition holds the FSM structure before building a Machine
type Definition struct {
	states      map[StateID]*State
	transitions []Transition
	initial     StateID
}

// NewDefinition creates a new FSM definition builder
func NewDefinition() *Definition {
	return &Definition{
		states:      make(map[StateID]*State),
		transitions: make([]Transition, 0),
	}
}

// State adds a normal state to the definition
func (d *Definition) State(id StateID, opts ...StateOption) *Definition {
	s := &State{
		ID:   id,
		Type: StateNormal,
	}
	for _, opt := range opts {
		opt(s)
	}
	d.states[id] = s
	return d
}

// ConditionState adds a condition pseudo-state that evaluates immediately on entry
func (d *Definition) ConditionState(id StateID, cond func(*Context) StateID, opts ...StateOption) *Definition {
	s := &State{
		ID:        id,
		Type:      StateCondition,
		Condition: cond,
	}
	for _, opt := range opts {
		opt(s)
	}
	d.states[id] = s
	return d
}

// JunctionState adds a junction pseudo-state (like condition but entry action runs first)
func (d *Definition) JunctionState(id StateID, cond func(*Context) StateID, opts ...StateOption) *Definition {
	s := &State{
		ID:        id,
		Type:      StateJunction,
		Condition: cond,
	}
	for _, opt := range opts {
		opt(s)
	}
	d.states[id] = s
	return d
}

// FinalState adds a terminal state with no outgoing transitions
func (d *Definition) FinalState(id StateID, opts ...StateOption) *Definition {
	s := &State{
		ID:   id,
		Type: StateFinal,
	}
	for _, opt := range opts {
		opt(s)
	}
	d.states[id] = s
	return d
}

// Transition adds a transition rule
func (d *Definition) Transition(from StateID, event EventID, to StateID, opts ...TransitionOption) *Definition {
	t := Transition{
		From:  from,
		Event: event,
		To:    to,
	}
	for _, opt := range opts {
		opt(&t)
	}
	d.transitions = append(d.transitions, t)
	return d
}

// AnyStateTransition adds a transition that can fire from any state
func (d *Definition) AnyStateTransition(event EventID, to StateID, opts ...TransitionOption) *Definition {
	return d.Transition(WildcardState, event, to, opts...)
}

// Initial sets the initial state
func (d *Definition) Initial(id StateID) *Definition {
	d.initial = id
	return d
}

// Validate checks the definition for errors
func (d *Definition) Validate() error {
	if d.initial == "" {
		return fmt.Errorf("no initial state defined")
	}

	if _, ok := d.states[d.initial]; !ok {
		return fmt.Errorf("initial state %q not defined", d.initial)
	}

	// Check all parent references are valid
	for id, state := range d.states {
		if state.Parent != "" {
			if _, ok := d.states[state.Parent]; !ok {
				return fmt.Errorf("state %q references undefined parent %q", id, state.Parent)
			}
		}
		if state.DefaultChild != "" {
			if _, ok := d.states[state.DefaultChild]; !ok {
				return fmt.Errorf("state %q references undefined default child %q", id, state.DefaultChild)
			}
		}
	}

	// Check all transition targets are valid
	for _, t := range d.transitions {
		if t.From != WildcardState {
			if _, ok := d.states[t.From]; !ok {
				return fmt.Errorf("transition from undefined state %q", t.From)
			}
		}
		if _, ok := d.states[t.To]; !ok {
			return fmt.Errorf("transition to undefined state %q", t.To)
		}
	}

	// Check condition/junction states have conditions
	for id, state := range d.states {
		if (state.Type == StateCondition || state.Type == StateJunction) && state.Condition == nil {
			return fmt.Errorf("condition/junction state %q has no condition function", id)
		}
	}

	// Check for cycles in parent hierarchy
	for id := range d.states {
		if err := d.checkParentCycle(id); err != nil {
			return err
		}
	}

	return nil
}

func (d *Definition) checkParentCycle(id StateID) error {
	visited := make(map[StateID]bool)
	current := id
	for current != "" {
		if visited[current] {
			return fmt.Errorf("cycle detected in parent hierarchy at state %q", current)
		}
		visited[current] = true
		state := d.states[current]
		if state == nil {
			break
		}
		current = state.Parent
	}
	return nil
}

// Build creates a Machine from the definition
func (d *Definition) Build(opts ...MachineOption) (*Machine, error) {
	if err := d.Validate(); err != nil {
		return nil, fmt.Errorf("invalid definition: %w", err)
	}

	// Auto-create transitions for states with TimeoutTarget
	for id, state := range d.states {
		if state.TimeoutTarget != "" {
			// Verify target state exists
			if _, ok := d.states[state.TimeoutTarget]; !ok {
				return nil, fmt.Errorf("state %q timeout target %q not defined", id, state.TimeoutTarget)
			}
			// Add automatic transition
			d.transitions = append(d.transitions, Transition{
				From:  id,
				Event: state.TimeoutEvent,
				To:    state.TimeoutTarget,
			})
		}
	}

	m := &Machine{
		definition:   d,
		currentState: "",
		events:       make(chan Event, 100),
		timers:       make(map[string]*timerEntry),
		logger:       Logger,
	}

	for _, opt := range opts {
		opt(m)
	}

	// Build parent-child relationships
	m.children = make(map[StateID][]StateID)
	for id, state := range d.states {
		if state.Parent != "" {
			m.children[state.Parent] = append(m.children[state.Parent], id)
		}
	}

	// Compute depth for each state
	m.depth = make(map[StateID]int)
	for id := range d.states {
		m.depth[id] = d.computeDepth(id)
	}

	return m, nil
}

func (d *Definition) computeDepth(id StateID) int {
	depth := 0
	current := id
	for current != "" {
		state := d.states[current]
		if state == nil || state.Parent == "" {
			break
		}
		depth++
		current = state.Parent
	}
	return depth
}
