package logging

// DiffInvites finds the invite a new member used by comparing invite use
// counts before and after the join. It returns false when that is
// ambiguous (several invites changed, a vanity URL, joins racing), so a log
// never names the wrong invite.
func DiffInvites(prev, cur []Invite) (Invite, bool) {
	before := make(map[string]Invite, len(prev))
	for _, inv := range prev {
		before[inv.Code] = inv
	}
	var found []Invite
	seen := make(map[string]bool, len(cur))
	for _, inv := range cur {
		seen[inv.Code] = true
		if inv.Uses > before[inv.Code].Uses {
			found = append(found, inv)
		}
	}
	// An invite that reached its maximum uses is deleted by the platform:
	// it disappears from the list instead of counting up.
	for _, inv := range prev {
		if !seen[inv.Code] && inv.MaxUses > 0 && inv.Uses+1 == inv.MaxUses {
			inv.Uses++
			found = append(found, inv)
		}
	}
	if len(found) != 1 {
		return Invite{}, false
	}
	return found[0], true
}
