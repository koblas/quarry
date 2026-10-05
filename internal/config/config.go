package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/platform/osreason"
)

// DefaultKeep is how many snapshots quarry keeps when snapshots.keep is unset.
const DefaultKeep = 12

// utf8BOM is the byte order mark some editors put first in a UTF-8 file.
var utf8BOM = []byte("\xef\xbb\xbf")

// Adjustment is one [[acb.adjustment]] item: amounts in cents, 0 when the item does not give that key.
type Adjustment struct {
	Security               string
	Date                   time.Time
	ReturnOfCapital        int64
	ReinvestedDistribution int64
}

// Config is quarry's settings after defaults are applied.
type Config struct {
	// Path is the absolute path of the config file, whether or not it exists.
	Path string
	// Keep is the number of snapshots to keep, at least 1.
	Keep int
	// QuickenPath is quicken.path with a leading "~/" expanded, "" when unset.
	QuickenPath string
	// Ignore is findings.ignore as written: file order, duplicates and any text kept, nil when unset.
	Ignore []string
	// Registered is accounts.registered as written: file order, duplicates and any text kept, nil when unset.
	Registered []string
	// NonRegistered is accounts.non-registered as written, like Registered.
	NonRegistered []string
	// Currency is reporting.currency: money.CAD when the file is missing or leaves it unset.
	Currency money.Currency
	// Adjustments is the [[acb.adjustment]] items in file order, nil when none.
	Adjustments []Adjustment
	// Warnings holds one line per unknown key, in file order.
	Warnings []string
	// WarningsAbsolute is Warnings with the config file named by its absolute path, for machine-readable output.
	WarningsAbsolute []string
}

// Load reads the config file at path, resolving "~/" against home. A missing
// or empty file yields the defaults. It refuses, with an error whose text is
// the whole line for the user, a file that cannot be read, is not valid TOML,
// or holds a bad value for a known key (snapshots.keep, quicken.path,
// findings.ignore, reporting.currency, accounts.registered, accounts.non-registered) or an account
// id listed in both account lists; unknown keys are Config.Warnings.
func Load(home, path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{Path: path, Keep: DefaultKeep, Currency: money.CAD}, nil
	}
	f := file{home: home, path: path, shown: homepath.Abbreviate(home, path), data: bytes.TrimPrefix(data, utf8BOM)}
	if err != nil {
		return Config{}, f.cannotRead(osreason.Reason(err), err)
	}

	return f.parse()
}
