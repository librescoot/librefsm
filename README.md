# librefsm

A lightweight, hierarchical finite state machine library for Go.

## Overview

librefsm is a flexible FSM library designed for building robust state machines with support for hierarchical states, guards, timers, and conditional transitions. Originally developed for the LibreScoot project to manage vehicle state logic.

## Features

- **Hierarchical States**: Organize states in parent-child relationships
- **Condition & Junction States**: Automatic state transitions based on runtime conditions
- **Guards**: Conditional transition execution
- **Timers**: Declarative timeout support with automatic cleanup
- **Event Queue**: Asynchronous and synchronous event processing
- **Type Safety**: Strongly-typed state and event IDs
- **State Callbacks**: Entry and exit actions for each state
- **Transition Actions**: Execute code during state transitions

## Installation

```bash
go get github.com/librescoot/librefsm
```

## Quick Start

```go
package main

import (
    "context"
    "github.com/librescoot/librefsm"
)

func main() {
    // Build state machine - string literals work directly
    def := librefsm.NewDefinition().
        State("off").
        State("on").
        Transition("off", "toggle", "on").
        Transition("on", "toggle", "off").
        Initial("off")

    // Create and start machine
    m, _ := def.Build()
    m.Start(context.Background())

    // Send events
    m.Send(librefsm.Event{ID: "toggle"}) // off -> on
    m.Send(librefsm.Event{ID: "toggle"}) // on -> off

    m.Stop()
}
```

## Advanced Example

With callbacks, typed constants, and timers:

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/librescoot/librefsm"
)

// Define typed constants for compile-time safety
const (
    StateOff  librefsm.StateID = "off"
    StateOn   librefsm.StateID = "on"
    EvToggle  librefsm.EventID = "toggle"
    EvTimeout librefsm.EventID = "timeout"
)

func main() {
    def := librefsm.NewDefinition().
        State(StateOff,
            librefsm.WithOnEnter(func(c *librefsm.Context) error {
                fmt.Println("Light off")
                return nil
            }),
        ).
        State(StateOn,
            librefsm.WithTimeout(5*time.Second, EvTimeout),
            librefsm.WithOnEnter(func(c *librefsm.Context) error {
                fmt.Println("Light on (auto-off in 5s)")
                return nil
            }),
        ).
        Transition(StateOff, EvToggle, StateOn).
        Transition(StateOn, EvToggle, StateOff).
        Transition(StateOn, EvTimeout, StateOff).
        Initial(StateOff)

    m, _ := def.Build()
    m.Start(context.Background())

    m.SendSync(librefsm.Event{ID: EvToggle}) // off -> on
    time.Sleep(6 * time.Second)               // wait for timeout -> off

    m.Stop()
}
```

### Simplified Timeout Transitions

Use `WithTimeoutTransition` to automatically create the timeout transition without manually defining the event:

```go
def := librefsm.NewDefinition().
    State(StateOff,
        librefsm.WithOnEnter(func(c *librefsm.Context) error {
            fmt.Println("Light off")
            return nil
        }),
    ).
    State(StateOn,
        // Automatically transitions to StateOff after 5 seconds
        librefsm.WithTimeoutTransition(5*time.Second, StateOff),
        librefsm.WithOnEnter(func(c *librefsm.Context) error {
            fmt.Println("Light on (auto-off in 5s)")
            return nil
        }),
    ).
    Transition(StateOff, EvToggle, StateOn).
    Transition(StateOn, EvToggle, StateOff).
    // No need to manually define the timeout transition
    Initial(StateOff)
```

### Timeout Callbacks with Retry

Timeout callbacks that return an error cause the timer to restart instead of sending the event. This enables retry patterns:

```go
def := librefsm.NewDefinition().
    State(StatePolling,
        librefsm.WithTimeout(400*time.Millisecond, EvPollComplete, func(c *librefsm.Context) error {
            // Try to read sensor
            if err := readSensor(); err != nil {
                // Return error to retry after another 400ms
                return err
            }
            // Return nil to send EvPollComplete and allow transition
            return nil
        }),
    ).
    State(StateReady).
    Transition(StatePolling, EvPollComplete, StateReady).
    Initial(StatePolling)
```

The timer keeps restarting until the callback succeeds (returns nil), at which point the event is sent.

## Documentation

See [example_test.go](example_test.go) for comprehensive examples including:
- Traffic light FSM with timers
- Vehicle state machine with hierarchical states and guards
- Condition and junction state usage

## Testing

```bash
go test -v
```

## License

This work is licensed under the [GNU Affero General Public License v3.0](LICENSE).

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

---

Part of the [LibreScoot](https://github.com/librescoot/librescoot) project
