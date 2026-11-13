// @vitest-environment node
// Reads protocol/schema.json off disk via import.meta.url — needs the node env
// (jsdom resolves import.meta.url to an http URL, which fileURLToPath rejects).
import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  VERSION,
  MESSAGE_FIELDS,
  DEFINITION_FIELDS,
  SEVERITIES,
  RATE_KINDS,
  EVENT_TYPES,
} from "./types";

// Load the canonical schema — the single source of truth shared with the Go end.
const schemaPath = fileURLToPath(
  new URL("../../../protocol/schema.json", import.meta.url),
);
const schema = JSON.parse(readFileSync(schemaPath, "utf8")) as {
  version: number;
  severities: string[];
  rate_kinds: string[];
  event_types: string[];
  messages: Record<string, { direction: string; fields: string[] }>;
  definitions: Record<string, { fields: string[] }>;
};

const sorted = (xs: string[]) => [...xs].sort();

describe("protocol/types consistency with schema.json", () => {
  it("VERSION matches schema version", () => {
    expect(VERSION).toBe(schema.version);
  });

  it("covers exactly the schema's message set", () => {
    expect(sorted(Object.keys(MESSAGE_FIELDS))).toEqual(
      sorted(Object.keys(schema.messages)),
    );
  });

  it.each(Object.keys(schema.messages))("message %s has matching fields", (name) => {
    expect(sorted(MESSAGE_FIELDS[name])).toEqual(sorted(schema.messages[name].fields));
  });

  it("covers exactly the schema's definition set", () => {
    expect(sorted(Object.keys(DEFINITION_FIELDS))).toEqual(
      sorted(Object.keys(schema.definitions)),
    );
  });

  it.each(Object.keys(schema.definitions))("definition %s has matching fields", (name) => {
    expect(sorted(DEFINITION_FIELDS[name])).toEqual(sorted(schema.definitions[name].fields));
  });

  it("severities match", () => {
    expect(sorted(SEVERITIES)).toEqual(sorted(schema.severities));
  });

  it("rate kinds match", () => {
    expect(sorted(RATE_KINDS)).toEqual(sorted(schema.rate_kinds));
  });

  it("event types match", () => {
    expect(sorted(EVENT_TYPES)).toEqual(sorted(schema.event_types));
  });
});
