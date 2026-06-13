package model

import "time"

// CertInfo is the TLS certificate metadata observed for a host. It is collected
// on demand (the --cert-check stage) and not persisted in the snapshot store —
// expiry alerts are computed against the live cert each run.
type CertInfo struct {
	Host   string    `json:"host"`
	Expiry time.Time `json:"expiry"` // leaf certificate NotAfter
}
