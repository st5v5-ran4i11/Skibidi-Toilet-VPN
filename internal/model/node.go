package model

import "time"

type NodeInfo struct {
	ID           string        `json:"id,omitempty" yaml:"id,omitempty"`
	DisplayName  string        `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Country      string        `json:"country,omitempty" yaml:"country,omitempty"`
	Raw          string        `json:"-" yaml:"-"`
	Protocol     string        `json:"protocol" yaml:"protocol"`
	Host         string        `json:"host" yaml:"host"`
	Port         string        `json:"port" yaml:"port"`
	UUID         string        `json:"uuid,omitempty" yaml:"uuid,omitempty"`
	Flow         string        `json:"flow,omitempty" yaml:"flow,omitempty"`
	Username     string        `json:"username,omitempty" yaml:"username,omitempty"`
	Password     string        `json:"password,omitempty" yaml:"password,omitempty"`
	Method       string        `json:"method,omitempty" yaml:"method,omitempty"`
	Security     string        `json:"security,omitempty" yaml:"security,omitempty"`
	SNI          string        `json:"sni,omitempty" yaml:"sni,omitempty"`
	Fingerprint  string        `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
	PublicKey    string        `json:"public_key,omitempty" yaml:"public_key,omitempty"`
	ShortID      string        `json:"short_id,omitempty" yaml:"short_id,omitempty"`
	Network      string        `json:"network,omitempty" yaml:"network,omitempty"`
	Path         string        `json:"path,omitempty" yaml:"path,omitempty"`
	HostHeader   string        `json:"host_header,omitempty" yaml:"host_header,omitempty"`
	AeadInsecure bool          `json:"aead_insecure,omitempty" yaml:"aead_insecure,omitempty"`
	Remark       string        `json:"remark,omitempty" yaml:"remark,omitempty"`
	Source       string        `json:"source,omitempty" yaml:"source,omitempty"`
	Sources      []string      `json:"sources,omitempty" yaml:"sources,omitempty"`
	Seen         int           `json:"seen,omitempty" yaml:"seen,omitempty"`
	TCPDelay     time.Duration `json:"-" yaml:"-"`
}

type FinalResult struct {
	Node            NodeInfo                 `json:"node"`
	Alive           bool                     `json:"alive"`
	Latency         time.Duration            `json:"latency"`
	SuccessRate     float64                  `json:"success_rate"`
	StabilityScore  int                      `json:"stability_score"`
	Score           int                      `json:"score"`
	Error           error                    `json:"-"`
	SuccessServices map[string]bool          `json:"success_services"`
	ServiceLatency  map[string]time.Duration `json:"service_latency,omitempty"`
	ServiceErrors   map[string]string        `json:"service_errors,omitempty"`
}
