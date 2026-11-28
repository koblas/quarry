# How to clean up your Quicken data

`quarry sync` records problems in the Quicken data as findings. Work through them one type at a time: fix each in Quicken, sync, and confirm it left the list. [Findings and the cleanup loop](../concepts/findings.md) explains why the fix is made in Quicken.

## 1. See what is open

```bash
quarry findings
```

lists the open findings, each with its id, the records it names and the fix to make.

## 2. Take one type at a time

```bash
quarry findings --type duplicate
```

Start with the types that change totals, in this order:

| Type | Fix in Quicken |
|---|---|
| `duplicate` | Delete the extra transaction. |
| `one-sided-transfer` | Re-enter it as a transfer between the two accounts. |
| `unlinked-transfer` | Make the pair one transfer between the two accounts. |
| `uncategorized` | Give that payee's splits a category. |
| `mixed-categories` | Pick one category for that payee's transactions. |
| `payee-variants` | Rename them to one payee and add a renaming rule. |
| `similar-categories` | Merge them into one. |
| `unused-category` | Check that no scheduled transaction or budget uses it, then delete it. |
| `shares-without-cost` | Open the Add Shares transaction and enter the cost, from the old broker's statement. |

`unclassified-account` is fixed in quarry's config file, not in Quicken: see [How to work out capital gains](capital-gains.md#1-classify-your-investment-accounts).

To work from a spreadsheet instead, export one row per record:

```bash
quarry findings --status all --csv > findings.csv
```

## 3. Sync and confirm

With the file still open in Quicken:

```bash
quarry sync
```

```bash
quarry findings --status fixed
```

lists the findings a later sync no longer found.

## 4. Ignore what is not a problem

Two real purchases of the same amount in the same week are listed as a `duplicate`. To keep a finding off the open list, copy its id from `quarry findings` into the config file, `~/Library/Application Support/quarry/config.toml`:

```toml
[findings]
ignore = ["duplicate:txn-4410+txn-4412", "uncategorized:payee-88"]
```

It stays ignored across syncs. Remove the id to list it again.

```bash
quarry findings --status ignored
```

lists what you have ignored.

## Do it with Claude

Ask Claude "What should I clean up in Quicken?". It goes through the types one at a time and waits while you make each fix. It changes nothing in Quicken, and writes to the config file only after showing you the lines and getting a yes.
