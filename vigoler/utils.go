package vigoler

import "strings"

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
