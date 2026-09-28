package importer_test

import "strconv"

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

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
