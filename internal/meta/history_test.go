package meta

import "testing"

// A page of older encrypted messages ends the conversation only when the server says so.
// A page that brought nothing to show, with more behind it, is followed at once.
func TestOlderVerdict(t *testing.T) {
	cases := []struct {
		name        string
		more        bool
		next, asked int64
		moved       bool
		verdict     bool
		cursor      int64
		again       bool
	}{
		{"the server has no more", false, 0, 1000, true, false, 0, false},
		{"no more, nothing new either", false, 0, 1000, false, false, 0, false},
		{"older messages arrived", true, 500, 1000, true, true, 500, false},
		{"nothing to show, more behind", true, 500, 1000, false, true, 500, true},
		{"the server stood still, nothing new", true, 1000, 1000, false, false, 0, false},
		{"the server stood still, but messages came", true, 1200, 1000, true, true, 0, false},
		{"no place named for the next page", true, 0, 1000, false, false, 0, false},
	}
	for _, c := range cases {
		verdict, cursor, again := olderVerdict(c.more, c.next, c.asked, c.moved)
		if verdict != c.verdict || cursor != c.cursor || again != c.again {
			t.Errorf("%s: got (%v, %d, %v), want (%v, %d, %v)", c.name, verdict, cursor, again, c.verdict, c.cursor, c.again)
		}
	}
}
