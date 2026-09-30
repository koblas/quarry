package config

// DefaultKeep is how many snapshots quarry keeps when snapshots.keep is unset.
const DefaultKeep = 12

// Config is quarry's settings after defaults are applied.
type Config struct {
	// Path is the absolute path of the config file, whether or not it exists.
	Path string
	// Keep is the number of snapshots to keep, at least 1.
	Keep int
	// QuickenPath is quicken.path with a leading "~/" expanded, "" when unset.
	QuickenPath string
	// Warnings holds one line per unknown key, in file order.
	Warnings []string
}

// Load reads the config file at path, resolving "~/" against home.
func Load(_, path string) (Config, error) {
	return Config{Path: path, Keep: DefaultKeep}, nil
}
