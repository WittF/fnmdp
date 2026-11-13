// Runtime smoke tests: mount pages/components that use HeroUI compound APIs and
// assert they render without throwing. Typecheck can't catch a wrong sub-component
// name (e.g. Select.Item vs ListBox.Item) — this does.
import { afterEach, describe, expect, it, vi, beforeAll } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Events } from "./Events";
import { Health } from "./Health";
import { EventItem } from "../components/EventRail";
import { StatusGrid } from "../components/StatusGrid";
import { $nodes, $projects, $screens, $skus } from "../store";
import { _setOffsetForTest } from "../lib/time";

beforeAll(() => {
  // jsdom lacks ResizeObserver (StatusGrid uses it) and fetch (history/events).
  (global as { ResizeObserver?: unknown }).ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      const body = url.includes("/api/events/page") ? { events: [], has_more: false } : [];
      return { ok: true, json: async () => body } as unknown as Response;
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  _setOffsetForTest(0);
  $nodes.set({});
});

describe("HeroUI page smoke", () => {
  it("renders Events filters (Select/Switch) without throwing", () => {
    const { container } = render(<Events />);
    expect(container).toBeTruthy();
  });

  it("renders Health table without throwing", () => {
    const { container } = render(<Health />);
    expect(container.querySelector("table")).toBeTruthy();
  });

  it("advances Health heartbeat ages without row data changes", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-06-20T00:00:10Z"));
    $nodes.set({
      n1: {
        node_id: "n1",
        node_name: "node-1",
        region: "hk",
        online: true,
        skew_ms: 0,
        in_flight: 0,
        soft_limited: 0,
        hard_blocked: 0,
        last_seen: Math.floor(Date.now() / 1000) - 10,
        assignments: [],
      },
    });

    render(<Health />);
    expect(screen.getByText("10s 前")).toBeTruthy();

    act(() => {
      vi.advanceTimersByTime(5000);
    });

    expect(screen.getByText("15s 前")).toBeTruthy();
  });

  it("renders an EventItem with a getV2 diff HoverCard without throwing", () => {
    const { container } = render(
      <MemoryRouter>
        <EventItem
          e={{
            id: "e1",
            event_type: "sku_onsale",
            severity: "notable",
            project_id: 100,
            screen_id: 10,
            sku_id: 1,
            old: { flag: 1, clickable: false, price_fen: 12800, num: 0 },
            new: { flag: 2, clickable: true, price_fen: 12800, num: 5 },
            price_fen: 12800,
            flag_text: "在售",
            node_id: "n1",
            server_ts: 1700000000,
            seq: 1,
          }}
        />
      </MemoryRouter>,
    );
    // The headline (event label) renders, and field changes make it a hover trigger.
    expect(container.textContent).toContain("开售");
    expect(container.querySelector(".hover-card__trigger")).toBeTruthy();
  });

  it("renders a full StatusGrid (Segment + cells) without throwing", () => {
    $projects.set({ "100": { project_id: 100 } as never });
    $screens.set({ "100:10": { project_id: 100, screen_id: 10 } as never });
    $skus.set({
      "100:10:1": {
        project_id: 100,
        screen_id: 10,
        sku_id: 1,
        desc: "VIP",
        flag: 2,
        flag_text: "在售",
        clickable: true,
        price_fen: 12800,
        num: 5,
        limit_num: 8,
        last_event_ts: 0,
        server_ts: 1700000000,
        updated_at: "",
      },
    });
    const { container } = render(
      <StatusGrid projectId={100} screenId={10} skuId={1} variant="full" />,
    );
    // grid renders a row of cells (role="img")
    expect(container.querySelectorAll('[role="img"]').length).toBeGreaterThan(0);
  });
});
