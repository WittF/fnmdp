import { beforeEach, describe, expect, it } from "vitest";
import type { Event as WsEvent } from "../protocol/types";
import { $feed, $projects, $screens, $skus, FEED_CAP, backfillEvents, onNewEvent, pushWsEvent } from "./index";
import { _setOffsetForTest, clockOffset } from "../lib/time";

function wsEvent(skuID: number, ts: number): WsEvent {
  return {
    type: "event",
    version: 8,
    event_type: "sku_restock",
    severity: "critical",
    project_id: 1001,
    screen_id: 10,
    sku_id: skuID,
    old: { flag: 4, clickable: false, price_fen: 8800, num: 0 },
    new: { flag: 2, clickable: true, price_fen: 8800, num: 1 },
    price_fen: 8800,
    flag_text: "在售",
    node_id: "node-1",
    server_ts: ts,
  };
}

describe("feed store", () => {
  beforeEach(() => {
    $feed.set([]);
    $projects.set({});
    $screens.set({});
    $skus.set({});
  });

  it("prepends newest-first and notifies listeners", () => {
    let notified = 0;
    const off = onNewEvent(() => notified++);
    pushWsEvent(wsEvent(1, 1000));
    pushWsEvent(wsEvent(2, 1001));
    off();
    pushWsEvent(wsEvent(3, 1002));
    const feed = $feed.get();
    expect(feed.map((e) => e.sku_id)).toEqual([3, 2, 1]);
    expect(notified).toBe(2); // listener unsubscribed before the 3rd
  });

  it("dedups identical events (REST backfill vs WS)", () => {
    pushWsEvent(wsEvent(1, 1000));
    pushWsEvent(wsEvent(1, 1000)); // same key
    expect($feed.get()).toHaveLength(1);
  });

  it("keeps same-second ops events from different nodes", () => {
    pushWsEvent({
      ...wsEvent(0, 1000),
      event_type: "risk_429",
      severity: "ops",
      project_id: 1001,
      screen_id: 0,
      sku_id: 0,
      old: null,
      new: null,
      node_id: "node-a",
      details: { status_code: 429, sec_request_id: "a" },
    });
    pushWsEvent({
      ...wsEvent(0, 1000),
      event_type: "risk_429",
      severity: "ops",
      project_id: 1001,
      screen_id: 0,
      sku_id: 0,
      old: null,
      new: null,
      node_id: "node-b",
      details: { status_code: 429, sec_request_id: "b" },
    });
    expect($feed.get()).toHaveLength(2);
  });

  it("keeps same-second state events when fingerprints differ", () => {
    pushWsEvent(wsEvent(1, 1000));
    pushWsEvent({
      ...wsEvent(1, 1000),
      old: { flag: 4, clickable: false, price_fen: 8800, num: 0 },
      new: { flag: 2, clickable: true, price_fen: 8800, num: 2 },
    });
    expect($feed.get()).toHaveLength(2);
  });

  it("caps the feed length, dropping the oldest", () => {
    for (let i = 0; i < FEED_CAP + 25; i++) pushWsEvent(wsEvent(i, 1000 + i));
    expect($feed.get()).toHaveLength(FEED_CAP);
    expect($feed.get()[0].sku_id).toBe(FEED_CAP + 24); // newest kept
  });

  it("backfill merges and sorts by server_ts desc without notifying", () => {
    let notified = 0;
    const off = onNewEvent(() => notified++);
    backfillEvents([
      {
        project_id: 1001, screen_id: 10, sku_id: 5,
        event_type: "sku_onsale", severity: "critical",
        old_flag: 1, new_flag: 2, old_clickable: false, new_clickable: true,
        old_price_fen: 0, price_fen: 8800, old_num: 0, new_num: 4,
        flag_text: "在售", server_ts: 900, node_id: "n", created_at: "",
      },
    ]);
    pushWsEvent(wsEvent(6, 2000)); // live push AFTER backfill should notify
    off();
    const feed = $feed.get();
    expect(feed[0].sku_id).toBe(6); // newer WS event on top
    expect(feed[1].sku_id).toBe(5);
    expect(notified).toBe(1); // backfill did not notify; only the WS push did
  });

  it("updates last_event_ts from live state events but ignores ops events", () => {
    $projects.set({ "1001": { project_id: 1001, last_event_ts: 900 } as never });
    $screens.set({ "1001:10": { project_id: 1001, screen_id: 10, last_event_ts: 900 } as never });
    $skus.set({ "1001:10:1": { project_id: 1001, screen_id: 10, sku_id: 1, last_event_ts: 900 } as never });

    pushWsEvent(wsEvent(1, 1000));
    expect($projects.get()["1001"].last_event_ts).toBe(1000);
    expect($screens.get()["1001:10"].last_event_ts).toBe(1000);
    expect($skus.get()["1001:10:1"].last_event_ts).toBe(1000);

    pushWsEvent({ ...wsEvent(1, 1100), event_type: "risk_429", severity: "ops", screen_id: 0, sku_id: 0 });
    expect($projects.get()["1001"].last_event_ts).toBe(1000);
  });

  it("never advances the global server clock from events (poll sources only)", () => {
    _setOffsetForTest(0);
    const farFuture = Math.floor(Date.now() / 1000) + 100_000;
    // A state event with a far-future server_ts must NOT ratchet serverNow forward…
    pushWsEvent(wsEvent(1, farFuture));
    expect(clockOffset()).toBe(0);
    // …nor an ops event (master wall-clock / node clock, ahead of the poll timeline).
    pushWsEvent({ ...wsEvent(0, farFuture), event_type: "risk_429", severity: "ops", screen_id: 0, sku_id: 0 });
    expect(clockOffset()).toBe(0);
    _setOffsetForTest(0);
  });
});
