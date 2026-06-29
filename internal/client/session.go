package client

import (
	"os"

	"github.com/seifreed/leroymerlin-cli/internal/config"
)

const sessionFile = "session.json"

// Session is the machine-managed cookie cache: a browser cookie (notably the
// DataDome clearance) used when anonymous uTLS reads still draw a bot challenge.
// Written by set-cookie, read by LoadAuth.
type Session struct {
	Cookie string `json:"cookie"`
}

// SaveSession persists the cookie cache (0600).
func SaveSession(s Session) error {
	return config.Save(sessionFile, s)
}

// LoadAuth loads any cached/configured cookie onto the client. Returns false
// when there is none. The name signals the load side effect; it is not a pure
// predicate.
func (c *Client) LoadAuth() bool {
	var s Session
	if err := config.Load(sessionFile, &s); err != nil && !os.IsNotExist(err) {
		c.logf("cookie cache could not be read (%v) — re-seed with `leroymerlin set-cookie`", err)
	}
	if s.Cookie == "" {
		if cfg, _ := config.LoadConfig(); cfg.Auth.Cookie != "" {
			s.Cookie = cfg.Auth.Cookie
		}
	}
	if s.Cookie != "" {
		c.Cookie = s.Cookie
	}
	return c.Cookie != ""
}
