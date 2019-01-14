package vigoler

import (
	"fmt"
	"strings"
)

type ArgumentError struct {
	stackTrack []byte
	argName    string
	argValue   interface{}
}

func (e *ArgumentError) Error() string {
	return fmt.Sprintf("argument error in arg %s that contain the value %s.\n%s", e.argName, e.argValue, e.stackTrack)
}
func (e *ArgumentError) Type() string {
	return "argument error"
}
func extractLineFromString(partString string) (string, string) {
	partString = strings.Replace(partString, "\r", "\n", 1)
	if i := strings.Index(partString, "\n"); i >= 0 {
		i++
		nextPartString := partString[i:]
		fullString := partString[:i]
		if fullString == "\n" {
			fullString = ""
		}
		return nextPartString, fullString
	}
	return partString, ""
}
