// Package protocol holds the hand-written Go types for every WebSocket message,
// verified against protocol/schema.json (the single source of truth) by
// consistency_test.go. The TS mirror lives in web/src/protocol.
package protocol

// Version is the protocol catalog version. It must equal schema.json#version;
// consistency_test.go enforces this. Bump both together when any message changes.
const Version = 8
