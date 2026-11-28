# config.toml

quarry reads `~/Library/Application Support/quarry/config.toml`, the directory that holds its store. The file is optional: a missing or empty file means every default below. A UTF-8 byte order mark is allowed.

A file that is not valid TOML, or holds a bad value for a known key, is refused before any command touches Quicken, with one line naming the key and what it must be. A key quarry does not know is a warning (`unknown key …; quarry ignores it`); keys match by exact spelling. quarry never writes the file.

## Keys

| Key | Type | Default | Description |
|---|---|---|---|
| `snapshots.keep` | whole number, 1 or more | `12` | How many snapshots `quarry sync` and `quarry snapshots prune` keep. |
| `quicken.path` | string: a full path or one starting with `~/` | unset | The `.quicken` file `quarry sync` snapshots when `--quicken` is not given. Unset: the only `.quicken` file in `~/Documents` or Quicken's Documents folder. |
| `reporting.currency` | `"CAD"`, `"USD"` or `"native"`, any letter case | `"CAD"` | The currency reports use when `--currency` is not given. `quarry acb` ignores it. |
| `findings.ignore` | list of finding ids, as strings | empty | Findings `quarry findings` leaves off the open list. |
| `accounts.registered` | list of account ids, as strings | empty | Registered investment accounts (TFSA, RRSP and the like); `quarry acb` leaves them out. |
| `accounts.non-registered` | list of account ids, as strings | empty | Non-registered investment accounts; `quarry acb` pools them. An id may be in only one of the two lists. |
| `acb.adjustment` | list of tables, each under its own `[[acb.adjustment]]` line | none | Return of capital and reinvested distributions from T3 slips, applied by `quarry acb`. |

Refusals that quote `accounts.registered` or `accounts.non-registered` mask the account ids.

## `[[acb.adjustment]]` items

Each item needs `acb.adjustment.security`, `acb.adjustment.date`, and at least one of the two amounts.

| Key | Type | Description |
|---|---|---|
| `acb.adjustment.security` | string, a security id such as `"sec-41"` | The security the adjustment applies to. |
| `acb.adjustment.date` | TOML local date, such as `2024-12-31` | The date of the distribution. |
| `acb.adjustment.return-of-capital` | amount in CAD above 0, at most two decimals | Return of capital, which lowers the ACB; any part above the ACB brings it to 0 and is a capital gain. |
| `acb.adjustment.reinvested-distribution` | amount in CAD above 0, at most two decimals | Reinvested distribution, which raises the ACB. |

## Example

```toml
[snapshots]
keep = 24

[quicken]
path = "~/Documents/Home.quicken"

[reporting]
currency = "CAD"

[findings]
ignore = ["duplicate:txn-4410+txn-4412", "uncategorized:payee-88"]

[accounts]
registered = ["acct-12", "acct-15"]
non-registered = ["acct-3"]

[[acb.adjustment]]
security = "sec-41"
date = 2024-12-31
return-of-capital = 12.34
reinvested-distribution = 5.00
```
