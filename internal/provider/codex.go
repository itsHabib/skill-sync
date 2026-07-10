package provider

import (
	"os"
	"path/filepath"
)

func init() {
	Register("codex", func(baseDir string) Provider {
		if baseDir == "" {
			home, _ := os.UserHomeDir()
			baseDir = filepath.Join(home, ".codex", "skills")
		}
		return &skillMDProvider{providerName: "codex", baseDir: baseDir}
	})
}
