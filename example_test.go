package librefsm_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/librescoot/librefsm"
)

// Example: Simple traffic light FSM
func Example_trafficLight() {
	const (
		stateRed    librefsm.StateID = "red"
		stateYellow librefsm.StateID = "yellow"
		stateGreen  librefsm.StateID = "green"

		evTimer librefsm.EventID = "timer"
	)

	def := librefsm.NewDefinition().
		State(stateRed,
			librefsm.WithTimeout(3*time.Second, evTimer),
			librefsm.WithOnEnter(func(c *librefsm.Context) error {
				fmt.Println("🔴 RED - Stop")
				return nil
			}),
		).
		State(stateGreen,
			librefsm.WithTimeout(3*time.Second, evTimer),
			librefsm.WithOnEnter(func(c *librefsm.Context) error {
				fmt.Println("🟢 GREEN - Go")
				return nil
			}),
		).
		State(stateYellow,
			librefsm.WithTimeout(1*time.Second, evTimer),
			librefsm.WithOnEnter(func(c *librefsm.Context) error {
				fmt.Println("🟡 YELLOW - Caution")
				return nil
			}),
		).
		Transition(stateRed, evTimer, stateGreen).
		Transition(stateGreen, evTimer, stateYellow).
		Transition(stateYellow, evTimer, stateRed).
		Initial(stateRed)

	m, _ := def.Build(
		librefsm.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	m.Start(ctx)
	<-ctx.Done()
	m.Stop()

	// Output:
	// 🔴 RED - Stop
}

// Example: Vehicle-like state machine with hierarchy
func Example_vehicleFSM() {
	// States
	const (
		stateInit      librefsm.StateID = "init"
		stateCondInit  librefsm.StateID = "cond_init"
		stateStandby   librefsm.StateID = "standby"
		stateParked    librefsm.StateID = "parked"
		stateDrive     librefsm.StateID = "drive"
		stateCondLock  librefsm.StateID = "cond_lock"
		stateShutdown  librefsm.StateID = "shutting_down"
	)

	// Events
	const (
		evInitComplete librefsm.EventID = "init_complete"
		evUnlock       librefsm.EventID = "unlock"
		evLock         librefsm.EventID = "lock"
		evGoToDrive    librefsm.EventID = "go_to_drive"
		evGoToPark     librefsm.EventID = "go_to_park"
		evTimeout      librefsm.EventID = "timeout"
	)

	// Application data
	type VehicleData struct {
		KickstandUp   bool
		DashboardReady bool
	}

	vehicle := &VehicleData{
		KickstandUp:   false,
		DashboardReady: true,
	}

	def := librefsm.NewDefinition().
		// Init state
		State(stateInit,
			librefsm.WithOnEnter(func(c *librefsm.Context) error {
				fmt.Println("→ Initializing...")
				// Simulate init complete
				c.Send(librefsm.Event{ID: evInitComplete})
				return nil
			}),
		).
		// Condition to determine initial state
		ConditionState(stateCondInit, func(c *librefsm.Context) librefsm.StateID {
			return stateStandby // Always start in standby
		}).
		// Standby - locked state
		State(stateStandby,
			librefsm.WithOnEnter(func(c *librefsm.Context) error {
				fmt.Println("→ Standby (locked)")
				return nil
			}),
		).
		// Parked - unlocked, kickstand down
		State(stateParked,
			librefsm.WithOnEnter(func(c *librefsm.Context) error {
				fmt.Println("→ Parked")
				return nil
			}),
		).
		// Drive - ready to ride
		State(stateDrive,
			librefsm.WithOnEnter(func(c *librefsm.Context) error {
				fmt.Println("→ Ready to Drive!")
				return nil
			}),
		).
		// Condition for lock - checks seatbox, etc.
		JunctionState(stateCondLock, func(c *librefsm.Context) librefsm.StateID {
			fmt.Println("→ Checking lock conditions...")
			return stateShutdown
		}).
		// Shutting down - transitioning to standby
		State(stateShutdown,
			librefsm.WithTimeout(100*time.Millisecond, evTimeout),
			librefsm.WithOnEnter(func(c *librefsm.Context) error {
				fmt.Println("→ Shutting down...")
				return nil
			}),
		).
		// Transitions
		Transition(stateInit, evInitComplete, stateCondInit).
		Transition(stateStandby, evUnlock, stateParked).
		Transition(stateParked, evGoToDrive, stateDrive,
			librefsm.WithGuard(func(c *librefsm.Context) bool {
				v := c.Data.(*VehicleData)
				return v.KickstandUp && v.DashboardReady
			}),
		).
		Transition(stateDrive, evGoToPark, stateParked).
		Transition(stateParked, evLock, stateCondLock).
		Transition(stateShutdown, evTimeout, stateStandby).
		Initial(stateInit)

	m, _ := def.Build(
		librefsm.WithData(vehicle),
		librefsm.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)

	// Simulate vehicle operations
	time.Sleep(10 * time.Millisecond)
	fmt.Printf("State: %s\n", m.CurrentState())

	// Unlock
	m.SendSync(librefsm.Event{ID: evUnlock})
	fmt.Printf("State: %s\n", m.CurrentState())

	// Try to drive (will fail - kickstand down)
	m.SendSync(librefsm.Event{ID: evGoToDrive})
	fmt.Printf("State: %s (kickstand down)\n", m.CurrentState())

	// Raise kickstand and try again
	vehicle.KickstandUp = true
	m.SendSync(librefsm.Event{ID: evGoToDrive})
	fmt.Printf("State: %s\n", m.CurrentState())

	// Park and lock
	m.SendSync(librefsm.Event{ID: evGoToPark})
	m.SendSync(librefsm.Event{ID: evLock})

	// Wait for shutdown
	time.Sleep(150 * time.Millisecond)
	fmt.Printf("State: %s\n", m.CurrentState())

	m.Stop()

	// Output:
	// → Initializing...
	// → Standby (locked)
	// State: standby
	// → Parked
	// State: parked
	// State: parked (kickstand down)
	// → Ready to Drive!
	// State: drive
	// → Parked
	// → Checking lock conditions...
	// → Shutting down...
	// → Standby (locked)
	// State: standby
}
