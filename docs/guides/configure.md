# How to configure quarry

quarry reads an optional config file, `~/Library/Application Support/quarry/config.toml`. Without it, every setting has its default. quarry never writes that file. Every key is on [config.toml](../reference/config.md).

## 1. Create the file

```bash
mkdir -p ~/Library/Application\ Support/quarry
touch ~/Library/Application\ Support/quarry/config.toml
```

## 2. Add the settings you need

=== "Keep more snapshots"

    ```toml
    [snapshots]
    keep = 24
    ```

    Takes effect after the next `quarry sync`, or now with `quarry snapshots prune`.

=== "Name the Quicken file"

    ```toml
    [quicken]
    path = "~/Documents/Home.quicken"
    ```

    `quarry sync` uses it when `--quicken` is not given.

=== "Report in USD"

    ```toml
    [reporting]
    currency = "USD"
    ```

    A command's `--currency` flag still overrides it. `quarry acb` always reports in CAD.

=== "Classify investment accounts"

    List each brokerage or retirement account's id from `quarry accounts`:

    ```toml
    [accounts]
    registered = [
      "acct-12",  # Questrade TFSA
      "acct-15",  # RBC RRSP
    ]
    non-registered = ["acct-3"]  # Questrade Margin
    ```

    `quarry acb` pools the non-registered accounts and leaves registered ones out. The `unclassified-account` finding leaves the list as soon as the account is listed, without a sync.

=== "Ignore a finding"

    Copy the id from `quarry findings`:

    ```toml
    [findings]
    ignore = ["duplicate:txn-4410+txn-4412", "uncategorized:payee-88"]
    ```

    It stays ignored across syncs; remove the id to list it again.

=== "Add a T3 adjustment"

    One item per return of capital or reinvested distribution, amounts in CAD:

    ```toml
    [[acb.adjustment]]
    security = "sec-41"
    date = 2024-12-31
    return-of-capital = 12.34
    reinvested-distribution = 5.00
    ```

## 3. Check it

```bash
quarry status
```

A malformed file, or a bad value for a known key, is refused with a line naming the key and what it must be, before any command touches Quicken. An unknown key is a warning; quarry ignores it.
