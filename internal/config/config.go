package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Monitor struct {
	Name         string        `yaml:"name" json:"name"`
	Type         string        `yaml:"type" json:"type"`
	Target       string        `yaml:"target" json:"target"`
	Interval     time.Duration `yaml:"interval" json:"interval"`
	Timeout      time.Duration `yaml:"timeout" json:"timeout"`
	ExpectStatus int           `yaml:"expect_status" json:"expect_status,omitempty"`
	Contains     string        `yaml:"contains" json:"contains,omitempty"`
	FailAfter    int           `yaml:"fail_after" json:"fail_after"`
}

type Config struct {
	Listen      string    `yaml:"listen"`
	DatabaseURL string    `yaml:"database_url"`
	Kafka       Kafka     `yaml:"kafka"`
	Monitors    []Monitor `yaml:"monitors"`
}

type Kafka struct {
	Brokers        []string `yaml:"brokers"`
	ChecksTopic    string   `yaml:"checks_topic"`
	IncidentsTopic string   `yaml:"incidents_topic"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse([]byte(os.ExpandEnv(string(raw))))
}

func Parse(raw []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return &c, c.Validate()
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.Kafka.ChecksTopic == "" {
		c.Kafka.ChecksTopic = "vigil.checks"
	}
	if c.Kafka.IncidentsTopic == "" {
		c.Kafka.IncidentsTopic = "vigil.incidents"
	}
	for i := range c.Monitors {
		m := &c.Monitors[i]
		if m.Type == "" {
			m.Type = "http"
		}
		if m.Interval == 0 {
			m.Interval = 30 * time.Second
		}
		if m.Timeout == 0 {
			m.Timeout = 10 * time.Second
		}
		if m.FailAfter == 0 {
			m.FailAfter = 2
		}
		if m.Type == "http" && m.ExpectStatus == 0 {
			m.ExpectStatus = 200
		}
	}
}

func (c *Config) Validate() error {
	seen := map[string]bool{}
	for _, m := range c.Monitors {
		switch {
		case m.Name == "":
			return fmt.Errorf("monitor with target %q has no name", m.Target)
		case seen[m.Name]:
			return fmt.Errorf("duplicate monitor name %q", m.Name)
		case m.Type != "http" && m.Type != "tcp":
			return fmt.Errorf("monitor %q: unknown type %q", m.Name, m.Type)
		case m.Target == "":
			return fmt.Errorf("monitor %q: target is required", m.Name)
		case m.Timeout >= m.Interval:
			return fmt.Errorf("monitor %q: timeout must be shorter than interval", m.Name)
		}
		seen[m.Name] = true
	}
	return nil
}
