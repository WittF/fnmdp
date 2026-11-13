# protocol — message contract (single source of truth)

`schema.json` is the **canonical** definition of every WebSocket message exchanged between
collector nodes, the central master, and the public frontend. The Go types in
`shared/protocol/messages.go` and the TS types in `web/src/protocol/types.ts` are **hand-written
on both ends** and verified against this file by consistency tests
(`shared/protocol/consistency_test.go` and `web/src/protocol/types.test.ts`). Change the schema
first; the tests will tell you which end drifted.

## Versioning

`schema.json#version` is a single integer for the whole catalog. Every message embeds the same
`version`. Bump it whenever any message's field set changes; both consistency tests assert the Go
`protocol.Version` const and the TS `VERSION` const equal `schema.json#version`.

Node authentication is carried by the WebSocket
`Authorization: Bearer <NODE_TOKEN>` handshake header, `assign` is a full assignment set,
`assign_delta` carries assignment objects for additions, and `poll_result.diff[]` has a
versioned nested shape in `definitions`. `poll_result` also carries full `project`
metadata plus changed `screens`, so the master can build project/screen state without
calling upstream itself.

## Message catalog (11)

Every message has `type` (the dispatch discriminator) + `version`.

### node → master

| `type` | purpose |
|---|---|
| `register` | node announces itself after authenticating with the WS handshake header |
| `heartbeat` | liveness + in-flight / rate-limit counters |
| `poll_result` | a normalized getV2 poll, carrying project metadata, changed screens, and the SKU-state `diff` |
| `rate_event` | a 429/412 rate-control hit (`kind` = `soft_limited`/`hard_blocked`, optional `sec_request_id`) |

### master → node

| `type` | purpose |
|---|---|
| `assign` | full assignment set (project + phase offset + interval) |
| `assign_delta` | incremental assignment change (added assignment objects / removed project ids) |
| `config` | global rate discipline (min interval, backoffs, UA) |

### master → frontend (public, unauthenticated)

| `type` | purpose |
|---|---|
| `snapshot` | full current state on connect (projects + nodes) |
| `state_update` | a single SKU's current authoritative state |
| `event` | a classified real-time event (`event_type` + `severity` + old/new fingerprint, with node/details context when present) |
| `node_status` | a node's online/health status |

## Enumerations (defined in schema, mirrored as Go/TS constants)

- **`severity`**: `critical` · `notable` · `info` · `ops`
- **`rate_event.kind`**: `soft_limited` (429) · `hard_blocked` (412)
- **`event_type`**: open enum; seed list in `schema.json#event_types`. Detection lives in the
  master L3 state machine. Grouped as SKU-level (`sku_*`), screen-level (`screen_*`,
  `day_soldout`), project-level (`project_*`, `wish_milestone`), and ops (`risk_*`, `node_*`,
  `monitor_*`).
