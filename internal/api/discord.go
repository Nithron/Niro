package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

// OAuthClient is the part of Discord's OAuth2 API the dashboard uses. It acts
// with the *user's* token only; the bot token never reaches the API process.
type OAuthClient interface {
	AuthorizeURL(state string) string
	Exchange(ctx context.Context, code string) (Token, error)
	CurrentUser(ctx context.Context, accessToken string) (Profile, error)
	CurrentUserGuilds(ctx context.Context, accessToken string) ([]UserGuild, error)
	// Revoke invalidates an access token at Discord (on logout).
	Revoke(ctx context.Context, accessToken string) error
}

type Token struct {
	AccessToken string
	ExpiresIn   time.Duration
}

type Profile struct {
	ID         snowflake.ID `json:"id,string"`
	Username   string       `json:"username"`
	GlobalName string       `json:"global_name,omitempty"`
	AvatarURL  string       `json:"avatar_url"`
}

type UserGuild struct {
	ID          snowflake.ID        `json:"id,string"`
	Name        string              `json:"name"`
	IconURL     string              `json:"icon_url,omitempty"`
	Owner       bool                `json:"owner"`
	Permissions discord.Permissions `json:"permissions"`
}

// scopes requested at login: who you are and which servers you're in.
var scopes = []discord.OAuth2Scope{discord.OAuth2ScopeIdentify, discord.OAuth2ScopeGuilds}

type discordOAuth struct {
	rest         rest.OAuth2
	http         *http.Client
	clientID     snowflake.ID
	clientSecret string
	redirectURI  string
}

func NewDiscordOAuth(clientID snowflake.ID, clientSecret, redirectURI string) OAuthClient {
	return &discordOAuth{
		rest:         rest.NewOAuth2(rest.NewClient("")),
		http:         &http.Client{Timeout: 10 * time.Second},
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
	}
}

func (d *discordOAuth) AuthorizeURL(state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", d.clientID.String())
	q.Set("scope", discord.JoinScopes(scopes))
	q.Set("redirect_uri", d.redirectURI)
	q.Set("state", state)
	q.Set("prompt", "none") // skip the consent screen for returning users
	return "https://discord.com/oauth2/authorize?" + q.Encode()
}

func (d *discordOAuth) Exchange(ctx context.Context, code string) (Token, error) {
	resp, err := d.rest.GetAccessToken(d.clientID, d.clientSecret, code, d.redirectURI, rest.WithCtx(ctx))
	if err != nil {
		return Token{}, fmt.Errorf("exchanging OAuth2 code: %w", err)
	}
	return Token{AccessToken: resp.AccessToken, ExpiresIn: resp.ExpiresIn}, nil
}

const revokeURL = "https://discord.com/api/v10/oauth2/token/revoke"

func (d *discordOAuth) Revoke(ctx context.Context, accessToken string) error {
	if accessToken == "" {
		return nil
	}
	form := url.Values{"token": {accessToken}, "token_type_hint": {"access_token"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(d.clientID.String(), d.clientSecret)
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("revoking token: Discord answered %s", resp.Status)
	}
	return nil
}

func (d *discordOAuth) CurrentUser(ctx context.Context, accessToken string) (Profile, error) {
	u, err := d.rest.GetCurrentUser(accessToken, rest.WithCtx(ctx))
	if err != nil {
		return Profile{}, err
	}
	p := Profile{ID: u.ID, Username: u.Username, AvatarURL: u.EffectiveAvatarURL(discord.WithSize(128))}
	if u.GlobalName != nil {
		p.GlobalName = *u.GlobalName
	}
	return p, nil
}

func (d *discordOAuth) CurrentUserGuilds(ctx context.Context, accessToken string) ([]UserGuild, error) {
	const pageSize = 200
	var out []UserGuild
	var after snowflake.ID
	for {
		page, err := d.rest.GetCurrentUserGuilds(accessToken, 0, after, pageSize, 0, false, rest.WithCtx(ctx))
		if err != nil {
			return nil, err
		}
		for _, g := range page {
			ug := UserGuild{ID: g.ID, Name: g.Name, Owner: g.Owner, Permissions: g.Permissions}
			if icon := g.IconURL(discord.WithSize(128)); icon != nil {
				ug.IconURL = *icon
			}
			out = append(out, ug)
			after = g.ID
		}
		if len(page) < pageSize {
			return out, nil
		}
	}
}
