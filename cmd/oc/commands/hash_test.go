package commands

import "testing"

// The hash test is here because everything downstream of it - the stdin form,
// the picker's address fields, the "is this a query or a heading" question -
// turns on getting it right, and a false positive sends a write somewhere
// nobody asked for.
func TestLooksLikeHash(t *testing.T) {
	yes := []string{"hOpOB7vIg6oiYz5sMVSlzGiJXic=", "2jmj7l5rSw0yVb/vlWAYkK/YBwk="}
	no := []string{
		"IsTask()",                      // a query
		`IsStatus("NEXT")`,              // a query with a string in it
		"{{ WorkTasks }}",               // a filter
		"",                              // nothing
		"hOpOB7vIg6oiYz5sMVSlzGiJXic",   // a hash with the padding lost
		"hOpOB7vIg6oiYz5sMVSlzGiJXic==", // one character too long
		"notbase64!!notbase64!!notb=",   // the right shape, not base64
	}
	for _, s := range yes {
		if !LooksLikeHash(s) {
			t.Errorf("LooksLikeHash(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if LooksLikeHash(s) {
			t.Errorf("LooksLikeHash(%q) = true, want false", s)
		}
	}
}
