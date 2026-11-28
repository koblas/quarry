# How to manage snapshots

Each `quarry sync` copies the Quicken file into a snapshot in `~/Library/Application Support/quarry/snapshots` and builds the store from it. [How quarry works](../concepts/how-quarry-works.md#snapshots) explains what a snapshot is for.

## List the snapshots

```bash
quarry snapshots
```

lists them newest first: when each was taken, its size, the Quicken file it came from, and which one the store was built from.

## Keep more or fewer

quarry keeps the newest 12. To change that, set `snapshots.keep` in `~/Library/Application Support/quarry/config.toml`:

```toml
[snapshots]
keep = 24
```

The new limit applies after the next `quarry sync`. To apply it now, see what would go:

```bash
quarry snapshots prune --dry-run
```

then delete:

```bash
quarry snapshots prune
```

The snapshot the store was built from is never deleted, even when it is older than the rest.

## Rebuild the store from an older snapshot

To answer from the data as it was on an earlier day, copy that snapshot's id from `quarry snapshots` and run:

```bash
quarry sync --from 20260927T143005Z
```

quarry rebuilds the store from that snapshot without reading Quicken, so Quicken can be closed. `quarry status` confirms which snapshot the store now holds.

To return to current data, open the file in Quicken and run `quarry sync`.

!!! note "Snapshots quarry cannot rebuild from"
    `quarry snapshots` marks a snapshot `schema differs` when quarry cannot import it, and `no manifest` when it cannot be used with `--from`.
