package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ProfileSettings struct {
	ParseWorkers      int      `yaml:"parser_workers"`
	FetcherWorkers    int      `yaml:"fetcher_workers"`
	TCPWorkers        int      `yaml:"tcp_workers"`
	TCPTimeoutMs      int      `yaml:"tcp_timeout_ms"`
	DeepNodesParallel int      `yaml:"deep_nodes_parallel"`
	DeepTimeoutSec    int      `yaml:"deep_timeout_sec"`
	MaxOutputLimit    int      `yaml:"max_output_limit"`
	StableOutputLimit int      `yaml:"stable_output_limit"`
	EnabledServices   []string `yaml:"enabled_services"`
}

type ServiceConfig struct {
	Enabled        bool     `yaml:"enabled"`
	ProbeURL       string   `yaml:"probe_url"`
	Domains        []string `yaml:"domains"`
	TargetNodes    int      `yaml:"target_nodes"`
	StandbyNodes   int      `yaml:"standby_nodes"`
	StickinessTTL  string   `yaml:"stickiness_ttl"`
	HealthInterval string   `yaml:"health_interval"`
}

func (s ServiceConfig) TTL() time.Duration {
	d, err := time.ParseDuration(s.StickinessTTL)
	if err != nil || d <= 0 {
		return 45 * time.Minute
	}
	return d
}
func (s ServiceConfig) Interval() time.Duration {
	d, err := time.ParseDuration(s.HealthInterval)
	if err != nil || d <= 0 {
		return 30 * time.Second
	}
	return d
}

type ServicesConfig struct {
	Services map[string]ServiceConfig `yaml:"services"`
}

type RecoveryConfig struct {
	MaxSourcesPerCycle int           `yaml:"max_sources_per_cycle"`
	MaxNodesAfterTCP   int           `yaml:"max_nodes_after_tcp"`
	MaxWorkingNodes    int           `yaml:"max_working_nodes"`
	RetryCooldown      time.Duration `yaml:"-"`
	RetryCooldownText  string        `yaml:"retry_cooldown"`
}

func (r RecoveryConfig) Cooldown() time.Duration {
	d, err := time.ParseDuration(r.RetryCooldownText)
	if err != nil || d <= 0 {
		return 10 * time.Minute
	}
	return d
}

type Config struct {
	App struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"app"`
	Paths struct {
		Data   string `yaml:"data"`
		Logs   string `yaml:"logs"`
		Output string `yaml:"output"`
	} `yaml:"paths"`
	NetworkMode    string            `yaml:"network_mode"`
	RunShareServer bool              `yaml:"run_share_server"`
	AllServices    map[string]string `yaml:"all_services"`
	Fetcher        struct {
		Timeout    int    `yaml:"timeout"`
		RawPath    string `yaml:"raw_path"`
		OutputFile string `yaml:"output_file"`
	} `yaml:"fetcher"`
	HomeProfile   ProfileSettings `yaml:"home_profile"`
	MobileProfile ProfileSettings `yaml:"mobile_profile"`

	Services ServicesConfig `yaml:"-"`
	Recovery RecoveryConfig `yaml:"-"`
}

func (c *Config) LoadRuntimeFiles(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "configs", "services.yaml"))
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &c.Services); err != nil {
		return err
	}

	data, err = os.ReadFile(filepath.Join(root, "configs", "background_sources.yaml"))
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &c.Recovery); err != nil {
		return err
	}
	return nil
}

func (c *Config) GetActiveProfile() ProfileSettings {
	if strings.EqualFold(c.NetworkMode, "mobile") {
		return c.MobileProfile
	}
	return c.HomeProfile
}

func (c *Config) GetDeepWorkersCount() int {
	n := c.GetActiveProfile().DeepNodesParallel
	if n <= 0 {
		return 2
	}
	if n > 8 {
		return 8
	}
	return n
}

func (c *Config) Validate() error {
	p := c.GetActiveProfile()
	if p.ParseWorkers <= 0 || p.FetcherWorkers <= 0 || p.TCPWorkers <= 0 {
		return errors.New("parser/fetcher/tcp workers must be > 0")
	}
	if p.TCPTimeoutMs <= 0 || p.DeepTimeoutSec <= 0 {
		return errors.New("tcp/deep timeouts must be > 0")
	}
	if len(p.EnabledServices) == 0 {
		return errors.New("at least one enabled service is required")
	}
	for _, service := range p.EnabledServices {
		if _, ok := c.AllServices[service]; !ok {
			return errors.New("unknown legacy service: " + service)
		}
	}
	return nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadSources(path string) (*SourcesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg SourcesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
