package crux

import "strings"

// HouseStyleHeader records the applied source in the kick itself, so runtime
// archives and dashboards describe the delivered prompt, even after config edits.
func HouseStyleHeader(source string) string { return "## HOUSE STYLE (source: " + source + ")" }

// HouseStyleSource returns the first house-style header's known source. Empty
// means a legacy or manually supplied prompt without house-style metadata.
func HouseStyleSource(message string) string {
	for _, line := range strings.Split(message, "\n") {
		for _, source := range []string{"repo", "hive", "built-in"} {
			if line == HouseStyleHeader(source) {
				return source
			}
		}
	}
	return ""
}
