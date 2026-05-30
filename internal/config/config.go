package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Xboard XboardConfig `json:"xboard"`
	XUI    XUIConfig    `json:"xui"`
	Sync   SyncConfig   `json:"sync"`
	Log    LogConfig    `json:"log"`
}

type XboardConfig struct {
	BaseURL        string `json:"base_url"`
	Token          string `json:"token"`
	NodeID         int64  `json:"node_id"`
	NodeType       string `json:"node_type"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type XUIConfig struct {
	BaseURL        string `json:"base_url"`
	APIToken       string `json:"api_token"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	TwoFactorCode  string `json:"two_factor_code"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	InsecureTLS    bool   `json:"insecure_tls"`
}

type SyncConfig struct {
	InboundIDs             []int  `json:"inbound_ids"`
	IntervalSeconds        int    `json:"interval_seconds"`
	TrafficIntervalSeconds int    `json:"traffic_interval_seconds"`
	DeleteStale            bool   `json:"delete_stale"`
	EmailPrefix            string `json:"email_prefix"`
	EnableTrafficReport    bool   `json:"enable_traffic_report"`
	StateFile              string `json:"state_file"`
}

type LogConfig struct {
	Level string `json:"level"`
}

func Load(path string) (*Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, err
	}
	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) setDefaults() {
	c.Xboard.BaseURL = strings.TrimRight(strings.TrimSpace(c.Xboard.BaseURL), "/")
	c.Xboard.NodeType = strings.ToLower(strings.TrimSpace(c.Xboard.NodeType))
	if c.Xboard.TimeoutSeconds == 0 {
		c.Xboard.TimeoutSeconds = 20
	}

	c.XUI.BaseURL = strings.TrimRight(strings.TrimSpace(c.XUI.BaseURL), "/")
	if c.XUI.TimeoutSeconds == 0 {
		c.XUI.TimeoutSeconds = 20
	}

	if c.Sync.IntervalSeconds == 0 {
		c.Sync.IntervalSeconds = 60
	}
	if c.Sync.TrafficIntervalSeconds == 0 {
		c.Sync.TrafficIntervalSeconds = c.Sync.IntervalSeconds
	}
	if c.Sync.EmailPrefix == "" {
		c.Sync.EmailPrefix = "xboard"
	}
	c.Sync.EmailPrefix = strings.Trim(c.Sync.EmailPrefix, " -_")
	if c.Sync.StateFile == "" {
		c.Sync.StateFile = "./xboard_link_3x-ui-state.json"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
}

func (c *Config) validate() error {
	if c.Xboard.BaseURL == "" {
		return errors.New("xboard.base_url is required")
	}
	if c.Xboard.Token == "" {
		return errors.New("xboard.token is required")
	}
	if c.Xboard.NodeID <= 0 {
		return errors.New("xboard.node_id must be greater than zero")
	}
	if c.Xboard.NodeType == "" {
		return errors.New("xboard.node_type is required")
	}
	if c.XUI.BaseURL == "" {
		return errors.New("xui.base_url is required")
	}
	if c.XUI.APIToken == "" && (c.XUI.Username == "" || c.XUI.Password == "") {
		return errors.New("xui.api_token or xui.username/password is required")
	}
	if len(c.Sync.InboundIDs) == 0 {
		return errors.New("sync.inbound_ids must contain at least one inbound id")
	}
	if c.Sync.EmailPrefix == "" {
		return errors.New("sync.email_prefix cannot be empty")
	}
	if c.Sync.IntervalSeconds < 5 {
		return fmt.Errorf("sync.interval_seconds must be >= 5")
	}
	if c.Sync.TrafficIntervalSeconds < 5 {
		return fmt.Errorf("sync.traffic_interval_seconds must be >= 5")
	}
	return nil
}

func (c XboardConfig) Timeout() time.Duration {
	return time.Duration(c.TimeoutSeconds) * time.Second
}

func (c XUIConfig) Timeout() time.Duration {
	return time.Duration(c.TimeoutSeconds) * time.Second
}

func (c SyncConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

func (c SyncConfig) TrafficInterval() time.Duration {
	return time.Duration(c.TrafficIntervalSeconds) * time.Second
}
