# Reference

Exact descriptions of every command, tool and setting. These pages describe; the [Guides](../guides/index.md) say what to do. `go test ./docs ./internal/config` fails when the code has a command, flag, MCP tool or config key these pages do not list.

| Page | Covers |
|---|---|
| [The CLI](cli.md) | Every `quarry` command and flag, the finding types and the snapshot statuses. |
| [MCP tools](mcp-tools.md) | Every tool `quarry mcp` serves, its parameters, limits, and the command it matches. |
| [config.toml](config.md) | Every key in the optional config file. |

The store's tables and views are described in the [store schema](https://github.com/koblas/quarry/blob/main/plugin/skills/quarry/references/schema.md), which is generated from the store and ships with the Claude skill.
