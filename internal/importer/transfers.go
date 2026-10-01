package importer

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/koblas/quarry/internal/store"
)

// transferLink is what pairTransfers needs about one imported split: its
// own ZQUICKENID, and its ZTRANSFER text ("" when NULL).
type transferLink struct {
	quickenID sql.NullInt64
	link      string
}

// pairTransfers builds the transfers rows for splits (links[i] belongs to
// splits[i]) and sets each leg's TransferAccountID.
func pairTransfers(splits []store.Split, links []transferLink, transactions []store.Transaction, accounts []store.Account) ([]store.Transfer, store.TransferCheck) {
	accountOf := make(map[string]string, len(transactions))
	for _, txn := range transactions {
		accountOf[txn.ID] = txn.AccountID
	}
	currencyOf := make(map[string]string, len(accounts))
	namedAccount := make(map[string]store.Account, len(accounts))
	for _, a := range accounts {
		currencyOf[a.ID] = a.Currency
		// Same-named accounts resolve to the lowest source id.
		if prev, ok := namedAccount[a.Name]; !ok || a.SourceID < prev.SourceID {
			namedAccount[a.Name] = a
		}
	}

	// One comparator orders both the walk and each pair's from/to legs: a
	// string comparison of split ids would put "split-10" before "split-9".
	bySourceID := func(i, j int) int { return cmp.Compare(splits[i].SourceID, splits[j].SourceID) }
	order := make([]int, len(splits))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, bySourceID)

	// A ZQUICKENID can repeat (an unset one reads as 0); the lowest source
	// id keeps it, since order is ascending.
	byQuickenID := make(map[int64]int)
	for _, i := range order {
		if q := links[i].quickenID; q.Valid {
			if _, seen := byQuickenID[q.Int64]; !seen {
				byQuickenID[q.Int64] = i
			}
		}
	}

	var rows []store.Transfer
	var check store.TransferCheck
	// Each split lands in at most one row, so the store's transfers primary
	// key never sees a duplicate: a used or self-linked counterpart leaves
	// the leg one-sided.
	used := make(map[int]bool, len(splits))
	for _, i := range order {
		link := links[i].link
		if link == "" || used[i] {
			continue
		}
		used[i] = true

		// A numeric link names the counterpart's ZQUICKENID among the
		// imported splits; any other link is an account name.
		quickenID, err := strconv.ParseInt(link, 10, 64)
		if err != nil {
			row, oneSided := nameFormLeg(&splits[i], link, namedAccount)
			rows = append(rows, row)
			check.OneSided = append(check.OneSided, oneSided)
			continue
		}
		j, ok := byQuickenID[quickenID]
		if !ok || used[j] {
			rows = append(rows, store.Transfer{ID: transferID(splits[i]), FromSplitID: splits[i].ID})
			check.OneSided = append(check.OneSided, store.OneSidedTransfer{ID: transferID(splits[i]), SourceID: splits[i].SourceID})
			continue
		}
		used[j] = true

		from, to := i, j
		if bySourceID(to, from) < 0 {
			from, to = to, from
		}
		row := pairLegs(&splits[from], &splits[to], accountOf, currencyOf)
		rows = append(rows, row)
		check.Paired++
		if row.CrossCurrency {
			check.CrossCurrency++
		}
	}
	return rows, check
}

// errTransferTotals is checkTransferTotals' error: the pairing counts disagree with the rows.
var errTransferTotals = errors.New("transfer pairing counts disagree with its rows")

// checkTransferTotals confirms every transfers row was counted as paired or
// one-sided, so the import_runs counts always add up to transfers_rows.
func checkTransferTotals(transfers []store.Transfer, check store.TransferCheck) error {
	if got := check.Paired + len(check.OneSided); got != len(transfers) {
		return fmt.Errorf("%w: counted %d paired and %d one-sided for %d transfer rows",
			errTransferTotals, check.Paired, len(check.OneSided), len(transfers))
	}
	return nil
}

// pairLegs links the from and to splits to each other's account and returns their
// transfers row, keyed by from.
func pairLegs(from, to *store.Split, accountOf, currencyOf map[string]string) store.Transfer {
	fromAccount, toAccount := accountOf[from.TransactionID], accountOf[to.TransactionID]
	from.TransferAccountID = &toAccount
	to.TransferAccountID = &fromAccount
	toID := to.ID
	return store.Transfer{
		ID: transferID(*from), FromSplitID: from.ID, ToSplitID: &toID,
		CrossCurrency: currencyOf[fromAccount] != currencyOf[toAccount],
	}
}

// nameFormLeg stores split as a one-sided transfer to the account named
// name, linking the account when an imported account carries that name.
func nameFormLeg(split *store.Split, name string, namedAccount map[string]store.Account) (store.Transfer, store.OneSidedTransfer) {
	oneSided := store.OneSidedTransfer{ID: transferID(*split), SourceID: split.SourceID, OtherAccount: &name}
	if acct, ok := namedAccount[name]; ok {
		id := acct.ID
		split.TransferAccountID = &id
		oneSided.OtherAccountID = &id
	}
	return store.Transfer{ID: oneSided.ID, FromSplitID: split.ID, OtherAccount: &name}, oneSided
}

// transferID is a transfer's stable id, derived from its from leg's source
// id.
func transferID(from store.Split) string {
	return fmt.Sprintf("xfer-%d", from.SourceID)
}
