// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sharedDocumentRun is one command and the exact bytes it must print.
type sharedDocumentRun struct {
	args   []string
	stdout string
	stderr string
}

// Byte-for-byte on purpose: JSONEq pins elsewhere stay green when a document's
// key order changes, and these three documents are shared across surfaces.
func Test_run_prints_the_sql_status_and_findings_documents_byte_for_byte(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, home string) sharedDocumentRun
	}{
		{name: "sql --json prints only the last statement's column and row", setup: func(t *testing.T, home string) sharedDocumentRun {
			t.Helper()
			syncAccountsFixture(t, home)
			return sharedDocumentRun{args: []string{"sql", "--json", "SELECT 1 AS a; SELECT 2 AS b"}, stdout: sqlLastStatementDocument}
		}},
		{name: "status --json with an unreadable config has a null ignored count and an absolute-path warning", setup: func(t *testing.T, home string) sharedDocumentRun {
			t.Helper()
			bundle := writeStatusFixtureBundle(t, home)
			syncBundle(t, bundle)
			writeConfig(t, home, "[snapshots]\nkeep = 0\n")
			snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
			snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
			raw, err := os.ReadFile(onlyFileWithSuffix(t, snapshotsDir, ".json"))
			require.NoError(t, err)
			var manifest struct {
				Snapshot struct {
					TakenAt string `json:"taken_at"`
					SHA256  string `json:"sha256"`
				} `json:"snapshot"`
			}
			require.NoError(t, json.Unmarshal(raw, &manifest))
			builtAt := storeBuiltAt(t)
			return sharedDocumentRun{
				args: []string{"status", "--json"},
				stdout: fmt.Sprintf(statusUnreadableConfigDocument, storePathUnder(home), builtAt, snapshotID(snapshotPath),
					snapshotPath, manifest.Snapshot.TakenAt, bundle.Dir, manifest.Snapshot.SHA256, configPath(home)),
				stderr: "quarry: warning: cannot tell which findings you ignored: " + configShown +
					": snapshots.keep must be a whole number of 1 or more, got 0; findings you ignored are counted as open\n",
			}
		}},
		{name: "findings --json lists a quoted unmatched ignore id escaped, with absolute path in the document", setup: func(t *testing.T, home string) sharedDocumentRun {
			t.Helper()
			syncStatusFindingsFixture(t, home)
			pinFirstFoundAt(t, home)
			writeConfig(t, home, "[findings]\nignore = [\"we\\\"ird\\\\id\"]\n")
			return sharedDocumentRun{
				args:   []string{"findings", "--json"},
				stdout: fmt.Sprintf(findingsUnmatchedIgnoreDocument, configPath(home)),
				stderr: "quarry: warning: " + configShown + `: findings.ignore lists "we\"ird\\id", which is not a finding in quarry's store; quarry skips it` + "\n",
			}
		}},
		{name: "sql --help prints the full help", setup: func(*testing.T, string) sharedDocumentRun {
			return sharedDocumentRun{args: []string{"sql", "--help"}, stdout: sqlHelpDocument}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			want := c.setup(t, home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), want.args, &stdout, &stderr)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, want.stdout, stdout.String())
			assert.Equal(t, want.stderr, stderr.String())
		})
	}
}

// storeBuiltAt is the built_at status --json prints for the store under $HOME.
func storeBuiltAt(t *testing.T) string {
	t.Helper()
	var stdout bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status", "--json"}, &stdout, &bytes.Buffer{}))
	var got struct {
		Store struct {
			BuiltAt string `json:"built_at"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	return got.Store.BuiltAt
}

const sqlLastStatementDocument = `{
  "columns": [
    {
      "name": "b",
      "type": "INTEGER"
    }
  ],
  "rows": [
    [
      2
    ]
  ],
  "row_count": 1,
  "limit": 500,
  "truncated": false,
  "warnings": []
}
`

// statusUnreadableConfigDocument takes, in order: store path, built_at, snapshot id, snapshot path,
// taken_at, source, sha256, config path.
const statusUnreadableConfigDocument = `{
  "store": {
    "path": %[1]q,
    "format_version": 7,
    "quarry_version": "(devel)",
    "built_at": %[2]q,
    "rows": {
      "accounts": 3,
      "categories": 0,
      "payees": 0,
      "tags": 0,
      "transactions": 4,
      "splits": 4,
      "split_tags": 0,
      "transfers": 2,
      "investment_transactions": 0,
      "securities": 0,
      "prices": 0
    }
  },
  "snapshot": {
    "id": %[3]q,
    "path": %[4]q,
    "taken_at": %[5]q,
    "source": %[6]q,
    "sha256": %[7]q
  },
  "dates": {
    "first": "2026-01-05",
    "last": "2026-03-20"
  },
  "balances": {
    "checked": 1,
    "never_reconciled": 1,
    "investment_accounts": 1
  },
  "splits": {
    "checked": 4
  },
  "shares": {
    "checked": 0
  },
  "transfers": {
    "paired": 1,
    "cross_currency": 1,
    "one_sided": 1
  },
  "findings": {
    "open": 2,
    "ignored": null,
    "fixed": 0,
    "new": 2,
    "newly_fixed": 0
  },
  "rates": {
    "first": "2026-01-02",
    "last": "2026-01-02",
    "fetch_error": null
  },
  "warnings": [
    "cannot tell which findings you ignored: %[8]s: snapshots.keep must be a whole number of 1 or more, got 0; findings you ignored are counted as open"
  ]
}
`

// findingsUnmatchedIgnoreDocument takes the absolute config path.
const findingsUnmatchedIgnoreDocument = `{
  "status": "open",
  "type": null,
  "counts": {
    "open": 4,
    "ignored": 0,
    "fixed": 0,
    "new": 4,
    "newly_fixed": 0
  },
  "findings": [
    {
      "id": "duplicate:txn-1+txn-2",
      "type": "duplicate",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Delete the extra one in Quicken, or ignore the pair if both are real",
      "items": [
        {
          "transaction_id": "txn-1",
          "split_id": null,
          "payee_id": null,
          "category_id": null,
          "date": "2026-08-03",
          "account_id": "acct-1",
          "account": "Chequing",
          "currency": "CAD",
          "payee": "Hydro One",
          "category": null,
          "amount": "-142.17",
          "other_account": null,
          "other_account_id": null,
          "transactions": null,
          "splits": null
        },
        {
          "transaction_id": "txn-2",
          "split_id": null,
          "payee_id": null,
          "category_id": null,
          "date": "2026-08-05",
          "account_id": "acct-1",
          "account": "Chequing",
          "currency": "CAD",
          "payee": "HYDRO ONE NETWORKS",
          "category": null,
          "amount": "-142.17",
          "other_account": null,
          "other_account_id": null,
          "transactions": null,
          "splits": null
        }
      ]
    },
    {
      "id": "uncategorized:payee-3",
      "type": "uncategorized",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Give this payee's splits a category in Quicken",
      "items": [
        {
          "transaction_id": "txn-3",
          "split_id": "split-3",
          "payee_id": null,
          "category_id": null,
          "date": "2026-03-01",
          "account_id": "acct-1",
          "account": "Chequing",
          "currency": "CAD",
          "payee": "Amazon",
          "category": null,
          "amount": "-10.00",
          "other_account": null,
          "other_account_id": null,
          "transactions": null,
          "splits": null
        }
      ]
    },
    {
      "id": "uncategorized:payee-4",
      "type": "uncategorized",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Give this payee's splits a category in Quicken",
      "items": [
        {
          "transaction_id": "txn-4",
          "split_id": "split-4",
          "payee_id": null,
          "category_id": null,
          "date": "2026-03-02",
          "account_id": "acct-1",
          "account": "Chequing",
          "currency": "CAD",
          "payee": "Costco",
          "category": null,
          "amount": "-20.00",
          "other_account": null,
          "other_account_id": null,
          "transactions": null,
          "splits": null
        }
      ]
    },
    {
      "id": "uncategorized:payee-5",
      "type": "uncategorized",
      "status": "open",
      "first_found_at": "2026-09-30T14:15:02Z",
      "fixed_at": null,
      "fix": "Give this payee's splits a category in Quicken",
      "items": [
        {
          "transaction_id": "txn-5",
          "split_id": "split-5",
          "payee_id": null,
          "category_id": null,
          "date": "2026-03-03",
          "account_id": "acct-1",
          "account": "Chequing",
          "currency": "CAD",
          "payee": "Shell",
          "category": null,
          "amount": "-30.00",
          "other_account": null,
          "other_account_id": null,
          "transactions": null,
          "splits": null
        }
      ]
    }
  ],
  "warnings": [
    "%[1]s: findings.ignore lists \"we\\\"ird\\\\id\", which is not a finding in quarry's store; quarry skips it"
  ]
}
`

const sqlHelpDocument = `Run one SQL query against quarry's store and print the result. The store is
opened read-only: a query cannot change it, read or write other files, or
load extensions. A query too large for memory may spill to a temporary
directory beside the store; quarry removes it when it exits.

Pass the query as one quoted argument, or - to read it from stdin. A query
that starts with - (such as a -- comment) goes after --:

  quarry sql -- "-- monthly totals
  SELECT ..."

Amounts are DECIMAL(18,2) in each account's own currency; negative is money
leaving the account. v_cash_flow and v_spending also carry each amount in
CAD and in USD (amount_cad and amount_usd; spent_cad and spent_usd),
converted per split at the Bank of Canada rate for its date and rounded to
the cent, as quarry spend and quarry cashflow convert; they are NULL for a
date before the first rate. v_account_balances has balance_cad and
balance_usd at today's rate. fx_rates holds one rate per business day:
usd_cad is the Canadian dollars in one US dollar. For spending and income,
query v_spending and v_cash_flow: they already leave out transfers between
your own accounts, Quicken's system categories, transactions excluded from
reports and accounts Quicken leaves out of reports, so their totals match
quarry spend and quarry cashflow. A transfer leg is any split named in
transfers.from_split_id or transfers.to_split_id.

Investment transactions are in investment_transactions, not in transactions,
v_cash_flow or v_spending, so dividends, interest and trades are not counted
as income or spending there. Their amount is DECIMAL(18,2) in the account's
own currency, negative when cash leaves the account; commission is
DECIMAL(18,4) in the account's own currency as Quicken recorded it (some
brokers charge fractions of a cent), NULL when there is none; shares is
DECIMAL(18,6) as Quicken recorded each transaction, negative when shares
leave. A split row carries split_new_shares and split_old_shares instead, so
a sum of shares is not a holding. prices holds each security's closing price
per day as Quicken recorded it, rounded to 6 decimals, in the security's
currency (securities.currency, NULL when Quicken records none).
holding_shares holds each account's count of each security, one row per span
of days it is unchanged and not zero (from_date through to_date, NULL while
still held), splits applied; these are the counts quarry sync checks against
Quicken. v_holdings has one row per holding per day held, through today:
price is the latest on or before date and price_date its day (NULL when
none), value is shares times price rounded to the cent, value_cad and
value_usd convert it at the rate for date, as quarry holdings does; filter
it by date. Neither includes cash in investment accounts. action is one of
add_shares, buy, capital_gain_long, capital_gain_short, dividend, interest,
margin_interest, misc_expense, misc_income, reinvest_dividend,
remove_shares, sell, split.

findings holds what sync found to clean up in Quicken, and finding_items
the transactions, splits, payees or categories each one is about;
fixed_at is set once a finding is no longer found. Which findings you
ignored is set in the config file, not the store: quarry findings shows
each one's status.

List the tables and views with: quarry sql "SHOW TABLES"

At most --limit rows are printed (500 unless set, every row with --csv);
when there are more, quarry says so on stderr. --limit 0 prints every row.
With --csv, an empty field is NULL and "" is an empty string, except in a
one-column result, where NULL is also written as "" so no row is blank.

Usage:
  quarry sql <query> [flags]

Examples:
  quarry sql "SELECT name, currency FROM accounts WHERE NOT closed"
  quarry sql --limit 0 --json - < monthly.sql
  quarry sql --csv "SELECT * FROM transactions" > transactions.csv

Flags:
      --csv       print the rows as CSV, with a header line
  -h, --help      help for sql
      --limit n   print at most n rows (500 unless set, every row with --csv; 0 prints every row)

Global Flags:
      --json   print the result as JSON on stdout
`
