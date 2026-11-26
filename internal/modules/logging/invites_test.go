package logging

import "testing"

func TestDiffInvites(t *testing.T) {
	prev := []Invite{{Code: "a", Uses: 1}, {Code: "b", Uses: 5}, {Code: "last", Uses: 9, MaxUses: 10}}
	cases := []struct {
		name string
		cur  []Invite
		want string // "" = unknown
	}{
		{"one invite counted up", []Invite{{Code: "a", Uses: 2}, {Code: "b", Uses: 5}, {Code: "last", Uses: 9, MaxUses: 10}}, "a"},
		{"new invite used", []Invite{{Code: "a", Uses: 1}, {Code: "b", Uses: 5}, {Code: "last", Uses: 9, MaxUses: 10}, {Code: "new", Uses: 1}}, "new"},
		{"last use deleted the invite", []Invite{{Code: "a", Uses: 1}, {Code: "b", Uses: 5}}, "last"},
		{"two joins raced", []Invite{{Code: "a", Uses: 2}, {Code: "b", Uses: 6}, {Code: "last", Uses: 9, MaxUses: 10}}, ""},
		{"nothing changed (vanity URL)", prev, ""},
		{"an unlimited invite was deleted", []Invite{{Code: "b", Uses: 5}, {Code: "last", Uses: 9, MaxUses: 10}}, ""},
	}
	for _, c := range cases {
		got, ok := DiffInvites(prev, c.cur)
		if (c.want == "") == ok || (ok && got.Code != c.want) {
			t.Errorf("%s: got %q (ok=%v), want %q", c.name, got.Code, ok, c.want)
		}
	}
}
