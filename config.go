package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	MachineID       string   `json:"machineId"`
	MachineName     string   `json:"machineName"`
	GameDirectories []string `json:"gameDirectories"`
	MaxScanDepth    int      `json:"maxScanDepth"`
	Language        string   `json:"language"`
	SteamAPIKey     string   `json:"steamApiKey"`
	VNDBEnabled     bool     `json:"vndbEnabled"`
	DLsiteEnabled   bool     `json:"dlsiteEnabled"`
}

func DefaultConfig() *Config {
	return &Config{
		MachineName:     "Default",
		GameDirectories: []string{".\\Games"},
		MaxScanDepth:    3,
		Language:        "zh-CN",
		VNDBEnabled:     true,
		DLsiteEnabled:   true,
	}
}

func LoadConfig(exeDir string) (*Config, error) {
	path := filepath.Join(exeDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			cfg.MachineID = generateMachineID()
			if saveErr := cfg.Save(exeDir); saveErr != nil {
				return cfg, nil
			}
			return cfg, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.MachineID == "" {
		cfg.MachineID = generateMachineID()
	}
	if cfg.MaxScanDepth == 0 {
		cfg.MaxScanDepth = 3
	}
	if cfg.Language == "" {
		cfg.Language = "zh-CN"
	}

	return &cfg, nil
}

func (c *Config) Save(exeDir string) error {
	path := filepath.Join(exeDir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func generateMachineID() string {
	host, _ := os.Hostname()
	return "machine-" + host
}
