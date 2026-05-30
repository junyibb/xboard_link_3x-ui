package config

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadPrefersXUINodeIDOverLegacySyncInboundIDs(t *testing.T) {
	cfg := loadConfigForTest(t, `{
		"xboard": {"base_url":"https://xboard.example.com","token":"token","node_id":3,"node_type":"vless"},
		"xui": {"base_url":"http://127.0.0.1:2053/panel","api_token":"token","node_id":3},
		"sync": {"inbound_ids":[1],"interval_seconds":60,"traffic_interval_seconds":60}
	}`)

	if got, want := cfg.TargetInboundIDs(), []int{3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetInboundIDs() = %v, want %v", got, want)
	}
}

func TestLoadSupportsCompatUppercaseNodeID(t *testing.T) {
	cfg := loadConfigForTest(t, `{
		"xboard": {"base_url":"https://xboard.example.com","token":"token","node_id":3,"node_type":"vless"},
		"xui": {"base_url":"http://127.0.0.1:2053/panel","api_token":"token","NodeID":3},
		"sync": {"inbound_ids":[],"interval_seconds":60,"traffic_interval_seconds":60}
	}`)

	if got, want := cfg.TargetInboundIDs(), []int{3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetInboundIDs() = %v, want %v", got, want)
	}
}

func TestLoadSupportsMultipleXUINodeIDs(t *testing.T) {
	cfg := loadConfigForTest(t, `{
		"xboard": {"base_url":"https://xboard.example.com","token":"token","node_id":3,"node_type":"vless"},
		"xui": {"base_url":"http://127.0.0.1:2053/panel","api_token":"token","node_ids":[1,3,3,0]},
		"sync": {"inbound_ids":[9],"interval_seconds":60,"traffic_interval_seconds":60}
	}`)

	if got, want := cfg.TargetInboundIDs(), []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetInboundIDs() = %v, want %v", got, want)
	}
}

func loadConfigForTest(t *testing.T, content string) *Config {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
