package client

// Session is the machine-managed cookie cache: the browser cookie (the DataDome
// clearance among the rest) that every read and write to the storefront runs on.
// Persistence belongs to the CLI/configuration boundary, not the HTTP client.
type Session struct {
	Cookie string `json:"cookie"`
	TabID  string `json:"tab_id,omitempty"`
}
