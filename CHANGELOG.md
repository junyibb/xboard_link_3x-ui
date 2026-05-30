# Changelog

## Unreleased

- Add `xui.node_id` / `xui.node_ids` as the preferred 3x-ui target inbound configuration.
- Keep `NodeID` / `NodeIDs` compatibility for existing hand-written configs.
- Keep `sync.inbound_ids` as a legacy fallback.
- Fix accidental attachment to the wrong 3x-ui inbound when only the legacy default inbound list was used.

## v0.1.0

Initial open-source version.

- Add Xboard UniProxy user sync.
- Add 3x-ui client create/update/delete support.
- Add compatibility for new `/panel/api/clients/*` API.
- Add compatibility fallback for legacy `/panel/api/inbounds/*` API.
- Add incremental traffic report to Xboard.
- Add local state file for traffic baseline.
- Add deployment and usage documentation.
