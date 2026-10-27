package crux

import "testing"

func TestHouseStyleSource(t *testing.T) {
	for _, source := range []string{"repo", "hive", "built-in"} {
		message := "SECURITY CANARY: opaque\n\n[agent:scanner]\n\n" + HouseStyleHeader(source) + "\n\nRules\n"
		if got := HouseStyleSource(message); got != source {
			t.Fatalf("got %q, want %q", got, source)
		}
	}
	for _, message := range []string{"", "legacy prompt", "## HOUSE STYLE (source: arbitrary)", "quoted ## HOUSE STYLE (source: repo)"} {
		if got := HouseStyleSource(message); got != "" {
			t.Fatalf("unrecognised header %q returned %q", message, got)
		}
	}
}
