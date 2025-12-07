package librefsm

// Event carries data through the state machine
type Event struct {
	ID      EventID
	Payload any // Optional typed payload
}

// Internal event IDs
const (
	eventEntry   EventID = "_entry"
	eventExit    EventID = "_exit"
	eventTimeout EventID = "_timeout"
)
