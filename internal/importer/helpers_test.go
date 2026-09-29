package importer_test

import (
	"database/sql"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// execOn runs query against the SQLite file at dataPath, for rows v9fixture.Builder cannot express.
func execOn(t *testing.T, dataPath, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite3", dataPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(query, args...)
	require.NoError(t, err)
}

func payeeIDs(fake *fakeStore) []string {
	ids := make([]string, len(fake.Rows.Payees))
	for i, p := range fake.Rows.Payees {
		ids[i] = p.ID
	}
	return ids
}

func tagIDs(fake *fakeStore) []string {
	ids := make([]string, len(fake.Rows.Tags))
	for i, tg := range fake.Rows.Tags {
		ids[i] = tg.ID
	}
	return ids
}

func transactionIDs(fake *fakeStore) []string {
	ids := make([]string, len(fake.Rows.Transactions))
	for i, txn := range fake.Rows.Transactions {
		ids[i] = txn.ID
	}
	return ids
}
