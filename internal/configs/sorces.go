package config

type SourcesConfig struct {
	Sources []Source `yaml:"sources"`
}
type Source struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	URL      string `yaml:"url"`
	Enabled  bool   `yaml:"enabled"`
	Protocol string `yaml:"protocol"`
}
