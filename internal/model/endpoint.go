package model

// Endpoint is a single URL discovered by crawling a live host (path + params).
type Endpoint struct {
	Target string `json:"target,omitempty"`
	Host   string `json:"host"`
	URL    string `json:"url"`
}
