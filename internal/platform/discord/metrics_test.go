package discord

import "testing"

func TestRestRoute(t *testing.T) {
	cases := map[string]string{
		"/api/v10/channels/123456789012345678/messages/223456789012345678":                        "/channels/:id/messages/:id",
		"/api/v10/guilds/123456789012345678/members/223456789012345678":                           "/guilds/:id/members/:id",
		"/api/v10/interactions/123456789012345678/aW50ZXJhY3Rpb24tdG9rZW4/callback":               "/interactions/:id/:token/callback",
		"/api/v10/webhooks/123456789012345678/c2VjcmV0LXRva2Vu/messages/@original":                "/webhooks/:id/:token/messages/@original",
		"/api/v10/applications/123456789012345678/guilds/223456789012345678/commands":             "/applications/:id/guilds/:id/commands",
		"/api/v10/users/@me/guilds/123456789012345678":                                            "/users/@me/guilds/:id",
		"/api/v10/channels/123456789012345678/messages/223456789012345678/reactions/thumbsup/@me": "/channels/:id/messages/:id/reactions/:x/@me",
		"/api/v10/invites/abcdef":        "/invites/:x",
		"/api/v10/guilds/1234/members/5": "/guilds/:x/members/:x",
		"/api/v10/channels/123456789012345678/messages/223456789012345678/reactions/%F0%9F%94%A5/@me": "/channels/:id/messages/:id/reactions/:x/@me",
	}
	for in, want := range cases {
		if got := restRoute(in); got != want {
			t.Errorf("restRoute(%s) = %s, want %s", in, got, want)
		}
	}
}
