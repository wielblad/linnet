//go:build windows

package main

import (
	"reflect"
	"testing"

	"lazyfmess/internal/store"
)

// Who gave the reactions: the same emoji from two people is one line with both names, two
// different ones are two lines.
func TestRxLines(t *testing.T) {
	name := func(id int64) string { return map[int64]string{1: "Ty", 2: "Marta", 3: "Olek"}[id] }
	got := rxLines([]store.Reaction{{ActorID: 2, Emoji: "👍"}, {ActorID: 3, Emoji: "❤"}, {ActorID: 1, Emoji: "👍"}}, name)
	want := []string{"👍  Marta, Ty", "❤  Olek"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
