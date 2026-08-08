package librefsm

// Event carries data through the state machine
type Event struct {
	ID      EventID
	Payload any // Optional typed payload
}
