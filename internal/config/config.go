package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type MetadataSource struct {
	Key      string            `json:"key"`
	Name     string            `json:"name"`
	Enabled  bool              `json:"enabled"`
	Settings map[string]string `json:"settings,omitempty"`
}

type Config struct {
	MachineID       string           `json:"machineId"`
	GameDirectories []string         `json:"gameDirectories"`
	MaxScanDepth    int              `json:"maxScanDepth"`
	Language        string           `json:"language"`
	Sources         []MetadataSource `json:"metadataSources"`
}

func Default() *Config {
	return &Config{
		GameDirectories: []string{".\\Games"},
		MaxScanDepth:    3,
		Language:        "zh-CN",
		Sources: []MetadataSource{
			{Key: "steam", Name: "Steam", Enabled: true},
			{Key: "vndb", Name: "VNDB (Visual Novel Database)", Enabled: true},
			{Key: "dlsite", Name: "DLsite", Enabled: true},
			{Key: "igdb", Name: "IGDB", Enabled: false},
		},
	}
}

func Load(exeDir string) (*Config, error) {
	path := filepath.Join(exeDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			cfg.MachineID = generateMachineID()
			if saveErr := cfg.Save(exeDir); saveErr != nil {
				return cfg, nil
			}
			return cfg, nil
		}
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	migrateLegacyFields(raw)

	remediated, _ := json.Marshal(raw)
	var cfg Config
	if err := json.Unmarshal(remediated, &cfg); err != nil {
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
	if len(cfg.Sources) == 0 {
		cfg.Sources = Default().Sources
	}

	return &cfg, nil
}

func migrateLegacyFields(raw map[string]json.RawMessage) {
	if _, hasSources := raw["metadataSources"]; hasSources {
		return
	}

	sources := []MetadataSource{
		{Key: "steam", Name: "Steam", Enabled: true},
	}

	vndbEnabled := true
	if v, ok := raw["vndbEnabled"]; ok {
		json.Unmarshal(v, &vndbEnabled)
		delete(raw, "vndbEnabled")
	}
	dlsiteEnabled := true
	if v, ok := raw["dlsiteEnabled"]; ok {
		json.Unmarshal(v, &dlsiteEnabled)
		delete(raw, "dlsiteEnabled")
	}

	sources = append(sources, MetadataSource{Key: "vndb", Name: "VNDB (Visual Novel Database)", Enabled: vndbEnabled})
	sources = append(sources, MetadataSource{Key: "dlsite", Name: "DLsite", Enabled: dlsiteEnabled})
	sources = append(sources, MetadataSource{Key: "igdb", Name: "IGDB", Enabled: false})

	data, _ := json.Marshal(sources)
	raw["metadataSources"] = data
}

func (c *Config) Save(exeDir string) error {
	path := filepath.Join(exeDir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) SourceSettings(key string) map[string]string {
	for _, s := range c.Sources {
		if s.Key == key {
			return s.Settings
		}
	}
	return nil
}

func generateMachineID() string {
	host, _ := os.Hostname()
	return "machine-" + host
}
