package event

import (
	"fmt"
)

type ErrInvalidEventName struct {
	EventName Name
}

func (err ErrInvalidEventName) Error() string {
	return fmt.Sprintf("event name '%s' does not comply with events API (does not match regular expression '%s')",
		err.EventName, AllowedEventFormat.String())
}
