# Recurring charges and anomalies

Use this for subscriptions and other repeating charges, when they started or changed price, and for unusually large charges. Recurring charges and anomalies have no SQL form: use `quarry recurring --json` and `quarry anomalies --json`, never `quarry sql`. SKILL.md sections 2 and 3 set the rules for every number you quote.

## Recurring charges

- `quarry recurring --json` lists charges that repeat on a schedule: the same payee and currency every week, month, quarter or year, at a steady amount. It looks at all history, with the rules of `quarry spend`: expense splits only, without transfers, refunds or accounts left out of reports.
- `--since` and `--until` choose which series to list: those running at any time in the period. Without them the period is January 1 this year up to today. `quarry recurring --since 2000 --json` lists all history.
- Charges dated after today are left out, even with a later `--until`.
- Bills whose amount changes most times, such as hydro, are not listed. Use `quarry spend --by payee` for those.
- A payee that charges in both CAD and USD has two series.
- A charge that comes off schedule starts the series again.

## Reading a series

Each entry of `series` has these fields.

- `payee` and `cadence`: `weekly`, `monthly`, `quarterly` or `annual`.
- `new`: true when the series' first charge falls in the period. Use it for "which subscriptions started this year"; answer from series with `new` true and name their `first_charge`.
- `first_charge` and `last_charge`: the dates of the first and latest charge. `charge_count` is how many charges the series has.
- `state`: `active` or `ended`. A series has ended when no charge has come for 14 days (weekly), 45 days (monthly), 120 days (quarterly) or 400 days (yearly).
- `amount` is the latest charge and `first_amount` the first. `per_year` is the latest amount times the charges in a year, for active series only; it is null for an ended series. `totals` has `per_year` for each currency.
- `price_changes`: a step of more than 5% from one charge to the next. Each entry has `date` (the later charge), `from`, `to` and `change_pct`, in the series' own currency. An empty list means the price never changed by more than that.
- `currency` is the reporting currency. `native_currency`, `native_amount` and `native_first_amount` are the series' own, and `price_changes` stay in them. A change in the exchange rate is never a price change.
- With `quarry recurring --currency native --json` nothing is converted. Never add CAD and USD together.

## Anomalies

- `quarry anomalies --json` lists charges that are unusually large: more than 2 times the median of the payee's earlier charges, when there are at least 3, or else more than 5 times the median of the category's earlier charges, when there are at least 10. Charges under 100.00 in their account's own currency are never listed.
- `--since` and `--until` choose which charges to list. Each is compared with every earlier charge, however old. Without them the period is January 1 this year up to today.
- `quarry anomalies --account "Visa Infinite" --json` lists only charges in that account; the payee's charges in other accounts still count as history.
- Each entry of `anomalies` has `amount`, `usual`, `times`, `baseline` and `earlier`. `usual` is the median of the earlier charges it was compared with, `times` is the charge as a multiple of it, `baseline` says whether that was the `payee`'s charges or the `category`'s, and `earlier` is how many charges made up the median.
- Charges are judged in their account's own currency. `native_amount` and `native_usual` are those; `amount` and `usual` are shown in the reporting currency at the rate on the charge's date.
- `checked` is how many charges the period held in the accounts listed. `not_judged` is how many of them, of 100.00 or more, had no baseline to compare with: an uncategorized or split charge from a payee with little history cannot be judged. A `not_judged` above zero means the list may be missing large charges; say so rather than calling the list complete.
- An empty `anomalies` list with a `not_judged` of zero is an answer: quarry found no unusually large charges for the period.
- Possible duplicates are listed by `quarry findings`, not here.
