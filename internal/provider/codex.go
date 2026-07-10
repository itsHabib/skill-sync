package provider

import (
	"fmt"
	"os"
	"path/filepath"
)

func init() {
	Register("codex", func(baseDir string) Provider {
		return newCodexProvider(baseDir, os.UserHomeDir)
	})
}

func newCodexProvider(baseDir string, userHomeDir func() (string, error)) Provider {
	if baseDir != "" {
		return &skillMDProvider{providerName: "codex", baseDir: baseDir}
	}

	home, err := userHomeDir()
	if err != nil {
		return &skillMDProvider{providerName: "codex", initErr: fmt.Errorf("codex: resolve user home: %w", err)}
	}
	if home == "" {
		return &skillMDProvider{providerName: "codex", initErr: fmt.Errorf("codex: resolve user home: empty path")}
	}
	return &skillMDProvider{providerName: "codex", baseDir: filepath.Join(home, ".codex", "skills")}
}
