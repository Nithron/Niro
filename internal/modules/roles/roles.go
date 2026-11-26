// Package roles is Niro's role panels: messages with buttons or a menu that
// let members pick their own roles. It is platform-agnostic; the Discord
// adapter renders panels and turns clicks into calls on Service.
//
// Security: a panel can only hand out ordinary roles. Managed roles,
// @everyone, roles with dangerous permissions and roles at or above Niro's
// own are refused when the panel is saved and again on every click.
package roles

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core/message"
)

// Style is how members pick roles.
type Style string

const (
	StyleButtons Style = "buttons"
	StyleSelect  Style = "select"
)

// Mode is how picks change roles.
type Mode string

const (
	ModeToggle Mode = "toggle" // add or remove each role
	ModeUnique Mode = "unique" // at most one role from the panel
	ModeVerify Mode = "verify" // add only; the panel never removes roles
)

// Limits (decision 25).
const (
	MaxPanels      = 25
	MaxRoles       = 25 // Discord: 25 buttons in 5 rows, or 25 menu options
	MaxName        = 100
	MaxLabel       = 80
	MaxDescription = 100
)

// PanelRole is one role on a panel.
type PanelRole struct {
	RoleID      snowflake.ID `json:"role_id"`
	Label       string       `json:"label"`
	Emoji       string       `json:"emoji,omitempty"`
	Description string       `json:"description,omitempty"` // select menus only
}

// Panel is a role panel.
type Panel struct {
	ID        int64
	GuildID   snowflake.ID
	Name      string
	ChannelID snowflake.ID
	MessageID snowflake.ID // 0 until posted
	Message   message.Document
	Style     Style
	Mode      Mode
	Roles     []PanelRole
	// MaxRoles limits how many of the panel's roles a member may hold in
	// toggle mode (0 = no limit).
	MaxRoles int
	// RequiredRoleID: only members with this role may use the panel (0 = anyone).
	RequiredRoleID snowflake.ID
	UpdatedBy      snowflake.ID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("invalid role panel")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Budget is what the panel's controls add to its message.
func (p Panel) Budget() message.Budget {
	if p.Style == StyleSelect {
		return message.Budget{Components: 2} // action row + menu
	}
	rows := (len(p.Roles) + 4) / 5
	return message.Budget{Components: rows + len(p.Roles)}
}

// Validate checks and normalizes the panel (not the roles' permissions:
// see CheckRoles).
func (p *Panel) Validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > MaxName {
		return invalid("the name must be 1 to %d characters", MaxName)
	}
	if p.ChannelID == 0 {
		return invalid("choose a channel")
	}
	if p.Style != StyleButtons && p.Style != StyleSelect {
		return invalid("style must be buttons or select")
	}
	if p.Mode != ModeToggle && p.Mode != ModeUnique && p.Mode != ModeVerify {
		return invalid("mode must be toggle, unique or verify")
	}
	if len(p.Roles) == 0 || len(p.Roles) > MaxRoles {
		return invalid("a panel has 1 to %d roles", MaxRoles)
	}
	seen := map[snowflake.ID]bool{}
	for i := range p.Roles {
		r := &p.Roles[i]
		r.Label = strings.TrimSpace(r.Label)
		r.Description = strings.TrimSpace(r.Description)
		switch {
		case r.RoleID == 0:
			return invalid("role %d: choose a role", i+1)
		case seen[r.RoleID]:
			return invalid("a role is on the panel twice")
		case r.Label == "" && (p.Style == StyleSelect || r.Emoji == ""):
			return invalid("role %d needs a label", i+1)
		case utf8.RuneCountInString(r.Label) > MaxLabel:
			return invalid("role %d: label is longer than %d characters", i+1, MaxLabel)
		case utf8.RuneCountInString(r.Description) > MaxDescription:
			return invalid("role %d: description is longer than %d characters", i+1, MaxDescription)
		case r.Emoji != "" && !message.IsEmoji(r.Emoji):
			return invalid("role %d: emoji must be a single Unicode emoji", i+1)
		}
		if p.Style == StyleButtons {
			r.Description = ""
		}
		seen[r.RoleID] = true
	}
	if p.RequiredRoleID != 0 && seen[p.RequiredRoleID] {
		return invalid("the required role can't also be on the panel")
	}
	if p.Mode != ModeToggle {
		p.MaxRoles = 0
	}
	if p.MaxRoles < 0 || p.MaxRoles > len(p.Roles) {
		return invalid("the role limit must be between 0 (none) and the number of roles")
	}
	if err := p.Message.Validate(p.Budget()); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

// RoleIDs are the panel's role IDs in order.
func (p Panel) RoleIDs() []snowflake.ID {
	out := make([]snowflake.ID, len(p.Roles))
	for i, r := range p.Roles {
		out[i] = r.RoleID
	}
	return out
}

// RoleInfo is what the platform says about a role.
type RoleInfo struct {
	ID       snowflake.ID
	Name     string
	Position int
	Managed  bool // bot, integration or booster role
	Everyone bool
	// Dangerous lists dangerous permissions the role grants (empty if none).
	Dangerous []string
}

// Problems with a role on a panel.
var (
	ErrRoleMissing   = errors.New("the role no longer exists")
	ErrRoleManaged   = errors.New("the role is managed by an integration or is @everyone")
	ErrRoleDangerous = errors.New("the role has dangerous permissions")
	ErrRoleTooHigh   = errors.New("the role is at or above Niro's highest role")
)

// RoleError says which role is the problem.
type RoleError struct {
	RoleID snowflake.ID
	Name   string
	Err    error
}

func (e *RoleError) Error() string {
	name := e.Name
	if name == "" {
		name = e.RoleID.String()
	}
	return fmt.Sprintf("role %s: %v", name, e.Err)
}

func (e *RoleError) Unwrap() error { return e.Err }

// CheckRole says whether Niro may hand out the role.
func CheckRole(id snowflake.ID, roles []RoleInfo, botTop int) error {
	i := slices.IndexFunc(roles, func(r RoleInfo) bool { return r.ID == id })
	if i < 0 {
		return &RoleError{RoleID: id, Err: ErrRoleMissing}
	}
	r := roles[i]
	switch {
	case r.Managed || r.Everyone:
		return &RoleError{RoleID: id, Name: r.Name, Err: ErrRoleManaged}
	case len(r.Dangerous) > 0:
		return &RoleError{RoleID: id, Name: r.Name, Err: fmt.Errorf("%w (%s)", ErrRoleDangerous, strings.Join(r.Dangerous, ", "))}
	case r.Position >= botTop:
		return &RoleError{RoleID: id, Name: r.Name, Err: ErrRoleTooHigh}
	}
	return nil
}

// ---- picks -------------------------------------------------------------------

// Change is what a pick does to a member's roles.
type Change struct {
	Add    []snowflake.ID
	Remove []snowflake.ID
}

// Empty reports whether nothing changes.
func (c Change) Empty() bool { return len(c.Add) == 0 && len(c.Remove) == 0 }

// Pick errors.
var (
	ErrNotOnPanel    = errors.New("that role is not on this panel")
	ErrNeedsRole     = errors.New("you need the required role to use this panel")
	ErrLimitReached  = errors.New("you already have the most roles this panel allows")
	ErrTooManyPicked = errors.New("too many roles picked")
)

func (p Panel) held(member []snowflake.ID) []snowflake.ID {
	var out []snowflake.ID
	for _, r := range p.Roles {
		if slices.Contains(member, r.RoleID) {
			out = append(out, r.RoleID)
		}
	}
	return out
}

func (p Panel) canUse(member []snowflake.ID) error {
	if p.RequiredRoleID != 0 && !slices.Contains(member, p.RequiredRoleID) {
		return ErrNeedsRole
	}
	return nil
}

// Press is a button press on roleID by a member holding member.
func (p Panel) Press(member []snowflake.ID, roleID snowflake.ID) (Change, error) {
	if !slices.Contains(p.RoleIDs(), roleID) {
		return Change{}, ErrNotOnPanel
	}
	if err := p.canUse(member); err != nil {
		return Change{}, err
	}
	has := slices.Contains(member, roleID)
	switch p.Mode {
	case ModeVerify:
		if has {
			return Change{}, nil
		}
		return Change{Add: []snowflake.ID{roleID}}, nil
	case ModeUnique:
		if has {
			return Change{Remove: []snowflake.ID{roleID}}, nil
		}
		others := slices.DeleteFunc(p.held(member), func(id snowflake.ID) bool { return id == roleID })
		return Change{Add: []snowflake.ID{roleID}, Remove: others}, nil
	default:
		if has {
			return Change{Remove: []snowflake.ID{roleID}}, nil
		}
		if p.MaxRoles > 0 && len(p.held(member)) >= p.MaxRoles {
			return Change{}, ErrLimitReached
		}
		return Change{Add: []snowflake.ID{roleID}}, nil
	}
}

// Choose is a select-menu submission: chosen are the panel roles the member
// wants; the menu replaces their panel roles with exactly these (verify mode
// only adds).
func (p Panel) Choose(member, chosen []snowflake.ID) (Change, error) {
	if err := p.canUse(member); err != nil {
		return Change{}, err
	}
	for _, id := range chosen {
		if !slices.Contains(p.RoleIDs(), id) {
			return Change{}, ErrNotOnPanel
		}
	}
	if (p.Mode == ModeUnique && len(chosen) > 1) || (p.MaxRoles > 0 && len(chosen) > p.MaxRoles) {
		return Change{}, ErrTooManyPicked
	}
	var c Change
	for _, id := range p.RoleIDs() {
		want, has := slices.Contains(chosen, id), slices.Contains(member, id)
		switch {
		case want && !has:
			c.Add = append(c.Add, id)
		case !want && has && p.Mode != ModeVerify:
			c.Remove = append(c.Remove, id)
		}
	}
	return c, nil
}

// MaxPicks is how many options the select menu allows.
func (p Panel) MaxPicks() int {
	switch {
	case p.Mode == ModeUnique:
		return 1
	case p.MaxRoles > 0:
		return p.MaxRoles
	}
	return len(p.Roles)
}

// ---- service -----------------------------------------------------------------

// Store persists panels.
type Store interface {
	CreatePanel(ctx context.Context, p Panel) (Panel, error)
	UpdatePanel(ctx context.Context, p Panel) (Panel, error)
	GetPanel(ctx context.Context, guildID snowflake.ID, id int64) (Panel, error)
	ListPanels(ctx context.Context, guildID snowflake.ID) ([]Panel, error)
	CountPanels(ctx context.Context, guildID snowflake.ID) (int, error)
	DeletePanel(ctx context.Context, guildID snowflake.ID, id int64) error
	SetPanelMessage(ctx context.Context, guildID snowflake.ID, id int64, channelID, messageID snowflake.ID) error
}

// Platform talks to the chat platform.
type Platform interface {
	Roles(ctx context.Context, guildID snowflake.ID) ([]RoleInfo, error)
	BotTopPosition(ctx context.Context, guildID snowflake.ID) (int, error)
	MemberRoles(ctx context.Context, guildID, userID snowflake.ID) ([]snowflake.ID, error)
	AddRole(ctx context.Context, guildID, userID, roleID snowflake.ID, reason string) error
	RemoveRole(ctx context.Context, guildID, userID, roleID snowflake.ID, reason string) error
	// PostPanel posts the panel and returns the message ID.
	PostPanel(ctx context.Context, p Panel) (snowflake.ID, error)
	// EditPanel edits the posted message; ErrMessageGone if it was deleted.
	EditPanel(ctx context.Context, p Panel) error
	DeleteMessage(ctx context.Context, channelID, messageID snowflake.ID) error
}

var (
	ErrNotFound     = errors.New("role panel not found")
	ErrTooManyPanel = errors.New("this server has the maximum number of role panels")
	ErrMessageGone  = errors.New("the panel's message was deleted")
	ErrModuleOff    = errors.New("the Role panels module is turned off")
	ErrTooNew       = errors.New("account too new for this panel")
)

// Deps are the service's collaborators.
type Deps struct {
	Store    Store
	Platform Platform
	// Enabled reports whether the module is on in a server.
	Enabled func(ctx context.Context, guildID snowflake.ID) (bool, error)
	// VerifyGate checks Guardian's minimum account age for verify-mode
	// panels; ok is false (with when it becomes old enough) if too new.
	VerifyGate func(ctx context.Context, guildID, userID snowflake.ID) (ok bool, when time.Time)
}

// Service manages panels and picks.
type Service struct{ d Deps }

func NewService(d Deps) *Service { return &Service{d: d} }

func (s *Service) checkRoles(ctx context.Context, p Panel) error {
	roles, err := s.d.Platform.Roles(ctx, p.GuildID)
	if err != nil {
		return err
	}
	top, err := s.d.Platform.BotTopPosition(ctx, p.GuildID)
	if err != nil {
		return err
	}
	for _, id := range p.RoleIDs() {
		if err := CheckRole(id, roles, top); err != nil {
			return err
		}
	}
	if p.RequiredRoleID != 0 && !slices.ContainsFunc(roles, func(r RoleInfo) bool { return r.ID == p.RequiredRoleID }) {
		return &RoleError{RoleID: p.RequiredRoleID, Err: ErrRoleMissing}
	}
	return nil
}

// Save creates (ID 0) or updates a panel and posts or edits its message.
func (s *Service) Save(ctx context.Context, p Panel) (Panel, error) {
	if err := p.Validate(); err != nil {
		return Panel{}, err
	}
	if err := s.checkRoles(ctx, p); err != nil {
		return Panel{}, err
	}
	var saved Panel
	if p.ID == 0 {
		n, err := s.d.Store.CountPanels(ctx, p.GuildID)
		if err != nil {
			return Panel{}, err
		}
		if n >= MaxPanels {
			return Panel{}, ErrTooManyPanel
		}
		saved, err = s.d.Store.CreatePanel(ctx, p)
		if err != nil {
			return Panel{}, err
		}
	} else {
		old, err := s.d.Store.GetPanel(ctx, p.GuildID, p.ID)
		if err != nil {
			return Panel{}, err
		}
		p.MessageID = old.MessageID
		if old.ChannelID != p.ChannelID && old.MessageID != 0 {
			_ = s.d.Platform.DeleteMessage(ctx, old.ChannelID, old.MessageID) // moved: the old message goes
			p.MessageID = 0
		}
		if saved, err = s.d.Store.UpdatePanel(ctx, p); err != nil {
			return Panel{}, err
		}
		saved.MessageID = p.MessageID
	}
	return s.publish(ctx, saved)
}

func (s *Service) publish(ctx context.Context, p Panel) (Panel, error) {
	if p.MessageID != 0 {
		err := s.d.Platform.EditPanel(ctx, p)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, ErrMessageGone) {
			return p, err
		}
	}
	id, err := s.d.Platform.PostPanel(ctx, p)
	if err != nil {
		return p, err
	}
	p.MessageID = id
	return p, s.d.Store.SetPanelMessage(ctx, p.GuildID, p.ID, p.ChannelID, id)
}

// Delete removes a panel and its message.
func (s *Service) Delete(ctx context.Context, guildID snowflake.ID, id int64) error {
	p, err := s.d.Store.GetPanel(ctx, guildID, id)
	if err != nil {
		return err
	}
	if p.MessageID != 0 {
		_ = s.d.Platform.DeleteMessage(ctx, p.ChannelID, p.MessageID) // already gone is fine
	}
	return s.d.Store.DeletePanel(ctx, guildID, id)
}

// Result is what a pick did.
type Result struct {
	Added   []snowflake.ID
	Removed []snowflake.ID
	// Failed are roles Niro may no longer hand out (with why).
	Failed []*RoleError
}

// PressButton handles a button press.
func (s *Service) PressButton(ctx context.Context, guildID snowflake.ID, panelID int64, userID, roleID snowflake.ID) (Result, error) {
	return s.pick(ctx, guildID, panelID, userID, func(p Panel, member []snowflake.ID) (Change, error) {
		return p.Press(member, roleID)
	})
}

// ChooseMenu handles a select-menu submission.
func (s *Service) ChooseMenu(ctx context.Context, guildID snowflake.ID, panelID int64, userID snowflake.ID, chosen []snowflake.ID) (Result, error) {
	return s.pick(ctx, guildID, panelID, userID, func(p Panel, member []snowflake.ID) (Change, error) {
		return p.Choose(member, chosen)
	})
}

func (s *Service) pick(ctx context.Context, guildID snowflake.ID, panelID int64, userID snowflake.ID, decide func(Panel, []snowflake.ID) (Change, error)) (Result, error) {
	on, err := s.d.Enabled(ctx, guildID)
	if err != nil {
		return Result{}, err
	}
	if !on {
		return Result{}, ErrModuleOff
	}
	p, err := s.d.Store.GetPanel(ctx, guildID, panelID)
	if err != nil {
		return Result{}, err
	}
	member, err := s.d.Platform.MemberRoles(ctx, guildID, userID)
	if err != nil {
		return Result{}, err
	}
	c, err := decide(p, member)
	if err != nil || c.Empty() {
		return Result{}, err
	}
	if p.Mode == ModeVerify && len(c.Add) > 0 && s.d.VerifyGate != nil {
		if ok, when := s.d.VerifyGate(ctx, guildID, userID); !ok {
			return Result{}, &TooNewError{When: when}
		}
	}
	// Check every role again: permissions or positions may have changed
	// since the panel was saved.
	roles, err := s.d.Platform.Roles(ctx, guildID)
	if err != nil {
		return Result{}, err
	}
	top, err := s.d.Platform.BotTopPosition(ctx, guildID)
	if err != nil {
		return Result{}, err
	}
	var res Result
	reason := fmt.Sprintf("Role panel: %s", p.Name)
	for _, id := range c.Remove {
		if err := CheckRole(id, roles, top); err != nil {
			res.Failed = append(res.Failed, err.(*RoleError))
			continue
		}
		if err := s.d.Platform.RemoveRole(ctx, guildID, userID, id, reason); err != nil {
			return res, err
		}
		res.Removed = append(res.Removed, id)
	}
	for _, id := range c.Add {
		if err := CheckRole(id, roles, top); err != nil {
			res.Failed = append(res.Failed, err.(*RoleError))
			continue
		}
		if err := s.d.Platform.AddRole(ctx, guildID, userID, id, reason); err != nil {
			return res, err
		}
		res.Added = append(res.Added, id)
	}
	return res, nil
}

// TooNewError: the account is younger than the verification gate allows.
type TooNewError struct{ When time.Time }

func (e *TooNewError) Error() string { return ErrTooNew.Error() }
func (e *TooNewError) Unwrap() error { return ErrTooNew }
