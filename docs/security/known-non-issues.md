# Known non-issues

Findings a security review reasoned about and judged not to be vulnerabilities, so later reviews do
not report them again. Each entry: the finding, where, why it is not an issue, the date and the
pull request or issue that settled it. A real finding becomes an issue labelled `security` instead.

- **Free text is not scanned for card or account numbers** (payees, memos, notes). The
  institution's download and Quicken already mask them, and quarry passes the text through as
  Quicken holds it. Settled by the PRD's Redaction decision (`docs/product/decisions.md`), 2026-09-27.
