# How to work out capital gains

`quarry acb` shows the adjusted cost base (ACB) of each security and the capital gains and losses realized each tax year, in CAD, the way the CRA defines them: one average cost per security, pooled across every non-registered account.

!!! warning "A worksheet, not a filing"
    The output is a worksheet to review with an accountant. Possible superficial losses are marked, not adjusted.

## 1. Classify your investment accounts

Quicken's file does not say which accounts are registered plans (RRSP, RRIF, TFSA, RESP, FHSA and the like), and `quarry acb` refuses to run until every brokerage and retirement account, open or closed, is classified. List the ones left:

```bash
quarry findings --type unclassified-account --status all
```

Add each account's id to one of two lists in `~/Library/Application Support/quarry/config.toml`:

```toml
[accounts]
registered = [
  "acct-12",  # Questrade TFSA
  "acct-15",  # RBC RRSP
]
non-registered = ["acct-3"]  # Questrade Margin
```

Run the `quarry findings` command again; it lists none when every account is classified. No sync is needed.

## 2. Enter missing costs

```bash
quarry findings --type shares-without-cost
```

lists shares that were moved or added into a non-registered account with no cost basis. `quarry acb` counts them at no cost, which overstates the gain. In Quicken, open each Add Shares transaction and enter the cost, then run `quarry sync`.

## 3. Add amounts from T3 slips

Return of capital and reinvested distributions change the ACB, and Quicken does not hold them. Add one item per amount, in CAD, to the config file. The security's id is in `quarry acb --json`, as `security_id`.

```toml
[[acb.adjustment]]
security = "sec-41"  # XEQT
date = 2024-12-31
return-of-capital = 12.34

[[acb.adjustment]]
security = "sec-41"  # XEQT
date = 2024-12-31
reinvested-distribution = 56.78
```

## 4. Read the worksheet

```bash
quarry acb
```

shows every security's ACB and the realized gains and losses per tax year.

```bash
quarry acb --year 2025
```

lists the sales in one tax year one by one.

```bash
quarry acb --security XEQT
```

shows one security's full history, by name, ticker or id.

Read the warnings under the output: they mark possible superficial losses and adjustments that did not apply.

## Do it with Claude

Ask Claude "What capital gains did we realize in 2025?". The first time, it asks which accounts are registered, shows you the config lines, and writes them only after a yes.
