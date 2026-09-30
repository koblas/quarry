package config

import (
	"errors"
	"io/fs"
	"os"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/osreason"
)

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

// Load reads the config file at path, resolving "~/" against home. A missing
// or empty file yields the defaults. It returns a *RefusalError when the file
// cannot be read, is not valid TOML, or holds a bad value for a known key;
// unknown keys are not an error but Config.Warnings.
func Load(home, path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{Path: path, Keep: DefaultKeep}, nil
	}
	f := file{home: home, path: path, shown: homepath.Abbreviate(home, path), data: data}
	if err != nil {
		return Config{}, f.refuse("cannot read "+f.shown+": "+osreason.Reason(err), err)
	}

	return f.parse()
}
