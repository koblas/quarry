package importer

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// securityIDFormat renders a security's quarry id from its ZSECURITY.Z_PK.
const securityIDFormat = "sec-%d"

// securitiesQuery reads each non-deleted security of one entity.
const securitiesQuery = `
SELECT Z_PK, ZNAME, ZTICKER, ZCURRENCY
FROM ZSECURITY
WHERE Z_ENT = ? AND COALESCE(ZDELETIONCOUNT, 0) = 0
ORDER BY Z_PK
`

// quotesQuery reads each non-deleted quote that has a security, a date and a price.
const quotesQuery = `
-- ZQUOTEDATE is cast to REAL: as TIMESTAMP the driver returns a Unix-epoch time.Time.
SELECT Z_PK, ZSECURITY, CAST(ZQUOTEDATE AS REAL), typeof(ZCLOSINGPRICE), CAST(ZCLOSINGPRICE AS TEXT)
FROM ZSECURITYQUOTE
WHERE Z_ENT = ? AND COALESCE(ZDELETIONCOUNT, 0) = 0
	AND ZSECURITY IS NOT NULL AND ZQUOTEDATE IS NOT NULL AND ZCLOSINGPRICE IS NOT NULL
ORDER BY Z_PK
`

// mapSecurities reads the non-deleted securities of securityEnt, and the same by source id.
// A NULL or "" ticker or currency is stored NULL; a NULL or "" name goes to off. With no entity in the snapshot nothing is read.
func mapSecurities(
	ctx context.Context, src Source, securityEnt int64, hasEntity bool, off *offenders,
) ([]store.Security, map[int64]store.Security, error) {
	if !hasEntity {
		return nil, nil, nil
	}
	var rows []store.Security
	bySource := make(map[int64]store.Security)
	err := src.QueryRows(ctx, securitiesQuery, []any{securityEnt}, func(scan func(dest ...any) error) error {
		var pk int64
		var name, ticker, currency sql.NullString
		if err := scan(&pk, &name, &ticker, &currency); err != nil {
			return err
		}
		if !name.Valid || name.String == "" {
			off.add(offender{class: classMissingValue, reason: reasonSecurityNoName(pk), name: fmt.Sprintf("(source id %d)", pk), sourceID: pk})
		}
		sec := store.Security{ID: fmt.Sprintf(securityIDFormat, pk), SourceID: pk, Name: name.String}
		if ticker.Valid && ticker.String != "" {
			sec.Ticker = &ticker.String
		}
		if currency.Valid && currency.String != "" {
			sec.Currency = &currency.String
		}
		rows = append(rows, sec)
		bySource[pk] = sec
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read securities: %w", err)
	}
	return rows, bySource, nil
}

// priceKey identifies one security's price on one UTC day.
type priceKey struct {
	security int64
	date     time.Time
}

// mapPrices keeps the highest-Z_PK quote of each (security, UTC day) of the imported securities.
// An unreadable price is added to off even when a later quote supersedes it.
func mapPrices(
	ctx context.Context, src Source, quoteEnt int64, hasEntity bool, securities map[int64]store.Security, off *offenders,
) ([]store.Price, error) {
	if !hasEntity {
		return nil, nil
	}
	kept := make(map[priceKey]store.Price)
	err := src.QueryRows(ctx, quotesQuery, []any{quoteEnt}, func(scan func(dest ...any) error) error {
		var pk, securityPK int64
		var seconds float64
		var typ, text string
		if err := scan(&pk, &securityPK, &seconds, &typ, &text); err != nil {
			return err
		}
		sec, ok := securities[securityPK]
		if !ok || sec.Name == "" { // a nameless security refuses the import; its quotes could only be named ""
			return nil
		}
		date := coreDataToDate(seconds)
		millionths, fault := parsePrice(typ, text)
		switch fault {
		case moneyNotANumber:
			off.add(offender{class: classNotANumber, reason: reasonPriceNotANumber(sec.Name, date.Format(dateLayout)), dated: true, date: date, account: sec.Name, sourceID: pk})
		case moneyTooLarge:
			off.add(offender{class: classTooLarge, reason: reasonPriceTooLarge(sec.Name, date.Format(dateLayout), text), dated: true, date: date, account: sec.Name, sourceID: pk})
		case moneyOK, moneyPrecision: // parsePrice never returns moneyPrecision
			kept[priceKey{securityPK, date}] = store.Price{SecurityID: sec.ID, SourceID: pk, Date: date, Price: millionths}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read prices: %w", err)
	}

	prices := make([]store.Price, 0, len(kept))
	for _, price := range kept {
		prices = append(prices, price)
	}
	sort.Slice(prices, func(i, j int) bool {
		if prices[i].SecurityID != prices[j].SecurityID {
			return prices[i].SecurityID < prices[j].SecurityID
		}
		return prices[i].Date.Before(prices[j].Date)
	})
	return prices, nil
}
