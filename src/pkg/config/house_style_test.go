package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHouseStyleConfigYAML(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("project:\n  style_file: /data/style.md\n  style: |\n    Keep changes focused.\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Project.StyleFile != "/data/style.md" || cfg.Project.Style != "Keep changes focused.\n" {
		t.Fatalf("house style keys not loaded: %+v", cfg.Project)
	}
}
