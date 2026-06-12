package model

// Finding is a single vulnerability/exposure detected by a scanner (nuclei)
// against a host.
type Finding struct {
	Host       string `json:"host"`
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
	Severity   string `json:"severity"`
	URL        string `json:"url,omitempty"`
}
