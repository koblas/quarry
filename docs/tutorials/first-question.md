# Your first question

In this tutorial you install quarry, take a first snapshot of your Quicken file, and answer a question from it. You need macOS 14 or later with Quicken Classic for Mac, Go, and the Xcode command-line tools (quarry links DuckDB through cgo).

## 1. Install quarry

```bash
go install github.com/koblas/quarry/cmd/quarry@latest
```

Check that your shell finds it:

```bash
quarry version
```

prints the installed release, such as `quarry v1.0.0`. If your shell does not find `quarry`, add `$(go env GOPATH)/bin` to your `PATH` and open a new terminal.

## 2. Take a snapshot

Open your file in Quicken, then run:

```bash
quarry sync
```

quarry copies the open file into a snapshot under `~/Library/Application Support/quarry/snapshots`, builds its store from it, and checks the store against Quicken's balances. With more than one `.quicken` file in `~/Documents`, name the one you want: `quarry sync --quicken ~/Documents/Home.quicken`.

## 3. Check what the store holds

```bash
quarry status
```

prints which snapshot the store was built from and what it holds.

## 4. Ask a question

```bash
quarry spend
```

prints this year's spending by category. Try `--by payee` or `--by month` for other groupings.

## Next

- Ask Claude instead of typing commands: [How to use quarry with Claude](../guides/use-with-claude.md).
- See what to tidy up in Quicken: [How to clean up your Quicken data](../guides/clean-up-quicken.md).
- Understand what a total includes: [How quarry counts money](../concepts/counting-rules.md).
- Every command and flag: [The CLI](../reference/cli.md).
