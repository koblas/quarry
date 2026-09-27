# Prior art

Frozen, verbatim copies of the two MIT-licensed projects `quarry` builds on. Nothing under this directory is edited; corrections go into `internal/quicken/v9/reference.sql` (schema) or into quarry's own code. Copyright and license text: `THIRD_PARTY_NOTICES` at the repository root and each project's `LICENSE` here.

| Directory | Source | Commit | Captured from |
| --- | --- | --- | --- |
| `dweekly/` | https://github.com/dweekly/quicken-mac-mcp | `119e2724a3cb0eab8729147d70b0d25f49326f2c` | Quicken 8.5 for Mac (per its `schema.md`) |
| `hardkoded/` | https://github.com/hardkoded/quicken-skills | `752107bd0c96512757559a590368f32b93cbca63` | a real Quicken for Mac 9.x file (per `test/fixture-schema.sql`) |

Files keep their upstream paths under each directory. `README.upstream.md` is each project's own README.

## What is used for what

- **Schema reference (embedded):** `hardkoded/test/fixture-schema.sql`, copied with a source header to `internal/quicken/v9/reference.sql`. Chosen over dweekly because it is 9.x and quarry targets Quicken Classic for Mac v9.
- **Schema semantics:** `dweekly/schema.md` (Core Data conventions, entity map, relationships).
- **SQL recipes:** `dweekly/src/tools/*.ts`, `dweekly/plugin/skills/quicken/references/*.md`, `hardkoded/plugins/quicken/skills/*/sql/*.sql`.
- **Skill layout:** `dweekly/plugin/skills/quicken/` (`SKILL.md` + `references/`).
- **Hygiene checks:** `hardkoded/plugins/quicken/skills/quicken-hygiene/`.
- **CSV exporter cross-check:** `dweekly/scripts/export_sovereign_csv.py`.
- **Test data:** `hardkoded/test/fixture-data.sql`.

## Schema reconciliation (dweekly 8.5 vs hardkoded 9.x)

Compared over every `Z`-prefixed table except `Z_METADATA` and `Z_MODELCACHE`, names only. dweekly: 83 tables, 1,829 columns. hardkoded: 82 tables, 1,835 columns. dweekly's `schema.md` lists `Z_15USERTAGS`, `Z_55USERTAGS`, `Z_1BUDGETS` and `Z_1DOWNLOADSESSIONS` twice (identical definitions).

**hardkoded wins every row** (it is the embedded reference).

Table only in dweekly: `ZTAXLINEITEM`.

Columns that differ only in the Core Data entity number baked into the name (Core Data renumbers entities between model versions, so these names are version-specific):

| Table | dweekly 8.5 | hardkoded 9.x |
| --- | --- | --- |
| ZATTACHMENT | Z79_TRANSACTION | Z78_TRANSACTION |
| ZCASHFLOWTRANSACTIONENTRY | Z79_PARENT | Z78_PARENT |
| ZCHECKPAYCHECK | Z80_TRANSACTION | Z79_TRANSACTION |
| ZCONNECTIVITYEVENTRESULT | Z79_TRANSACTION | Z78_TRANSACTION |
| ZIFSBILLPAYPAYMENT | Z80_TRANSACTION | Z79_TRANSACTION |
| ZOFXEMAIL | Z79_TRANSACTION | Z78_TRANSACTION |
| ZQERROR | Z79_TRANSACTION | Z78_TRANSACTION |
| ZQUICKPAYPAYMENT | Z80_TRANSACTION | Z79_TRANSACTION |
| ZRECONCILEDTRANSACTION | Z79_ORIGINALTRANSACTION | Z78_ORIGINALTRANSACTION |

Columns only in dweekly 8.5: `ZSALESTAXRATE.ZTAXAGENCYNAME`.

Columns only in hardkoded 9.x:

- `ZACCOUNT`: `ZCOLORSTRING`, `ZCOSTBASISALGORITHMCOMMODITY`, `ZCOSTBASISALGORITHMCRYPTO`, `ZCOSTBASISALGORITHMETF`, `ZDISPLAYVALUELINESHAVECOMPLETEDINITIALSETUP`
- `ZBUSINESS`: `ZACCOUNTINGBASISSTRING`
- `ZFIPOSITION`: `ZASSETCLASSPERCENTAGEMIDCAPSTOCK`
- `ZIFSBILLPAYBILLERACCOUNT`: `ZSTATUS`
- `ZINVOICE`: `ZPAYMENTMETHOD_ACH`, `ZPAYMENTMETHOD_CARD`
- `ZSECURITY`: `ZASSETCLASSPERCENTAGEMIDCAPSTOCK`, `ZCURRENCY`
- `ZTRANSACTION`: `ZAUTOUPDATEEBILLDATE`

Consequence for recipes ported from dweekly: any SQL naming an entity-numbered column (`Z79_PARENT` and the like) must use the 9.x name.
