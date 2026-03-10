package provider

func init() {
	Register("directory", func(baseDir string) Provider {
		return &skillMDProvider{providerName: "directory", baseDir: baseDir}
	})
}
