// Black-box seam: KnownKeys lets the config_test package check the reference page against the keys Load reads.
package config

import "strings"

// KnownKeys is every key path Load reads, tables included, dotted as the reference page writes them.
func KnownKeys() []string {
	keys := make([]string, 0, len(knownKeys))
	for _, key := range knownKeys {
		keys = append(keys, strings.Join(key, "."))
	}

	return keys
}
