package orgs

import "testing"

// A capture's priority and properties are written, the way org writes them.
func TestCapturePriorityAndProperties(t *testing.T) {
	kws := []string{"TODO", "NEXT", "DONE"}
	cases := map[[2]string]string{
		{"TODO Call Bob", "a"}:     "TODO [#A] Call Bob",
		{"Call Bob", "B"}:          "[#B] Call Bob",
		{"TODO", "C"}:              "TODO [#C]",
		{"TODO [#A] Call Bob", "B"}: "TODO [#A] Call Bob",
		{"Call Bob", ""}:           "Call Bob",
		{"Call Bob", "high"}:       "Call Bob",
	}
	for in, want := range cases {
		if got := withPriority(in[0], in[1], kws); got != want {
			t.Errorf("withPriority(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}

	props := map[string]string{"effort": "1d", "ID": "x1"}
	if got, want := withProperties("Some text", props), ":PROPERTIES:\n:EFFORT:   1d\n:ID:       x1\n:END:\nSome text\n"; got != want {
		t.Errorf("new drawer:\n%q\nwant\n%q", got, want)
	}
	if got, want := withProperties("SCHEDULED: <2026-10-12 Mon>\nBody", map[string]string{"ID": "x1"}),
		"SCHEDULED: <2026-10-12 Mon>\n:PROPERTIES:\n:ID:       x1\n:END:\nBody\n"; got != want {
		t.Errorf("after planning:\n%q\nwant\n%q", got, want)
	}
	if got, want := withProperties(":PROPERTIES:\n:ID: mine\n:END:\nBody", props),
		":PROPERTIES:\n:ID: mine\n:EFFORT:   1d\n:END:\nBody\n"; got != want {
		t.Errorf("merged, template wins:\n%q\nwant\n%q", got, want)
	}
	if got := withProperties("Body", nil); got != "Body" {
		t.Errorf("no props should leave content alone, got %q", got)
	}
}
