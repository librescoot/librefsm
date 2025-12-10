package librefsm

// Transition defines a state change rule
type Transition struct {
	From   StateID // Source state (or "*" for any-state)
	Event  EventID // Triggering event
	To     StateID // Target state
	Guard  func(ctx *Context) bool  // Optional: must return true to take transition
	Action func(ctx *Context) error // Optional: runs during transition
}

// WildcardState matches any state in transition rules
const WildcardState StateID = "*"

// TransitionOption is a functional option for configuring a Transition
type TransitionOption func(*Transition)

// WithGuard sets a guard condition for the transition
func WithGuard(fn func(*Context) bool) TransitionOption {
	return func(t *Transition) {
		t.Guard = fn
	}
}

// WithGuards sets multiple guard conditions that must ALL pass (AND logic)
func WithGuards(guards ...func(*Context) bool) TransitionOption {
	return func(t *Transition) {
		t.Guard = func(ctx *Context) bool {
			for _, g := range guards {
				if !g(ctx) {
					return false
				}
			}
			return true
		}
	}
}

// WithAction sets an action to execute during the transition
func WithAction(fn func(*Context) error) TransitionOption {
	return func(t *Transition) {
		t.Action = fn
	}
}
