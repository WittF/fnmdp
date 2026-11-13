import { describe, expect, it, vi } from "vitest";
import {
  _setOffsetForTest,
  clockOffset,
  formatAge,
  formatCountdown,
  formatDate,
  secondsUntil,
  serverNow,
  shouldShowDateBesideName,
  syncServerClock,
} from "./time";

describe("time (server clock basis)", () => {
  it("formats countdowns as Nd HH:MM:SS", () => {
    expect(formatCountdown(0)).toBe("00:00:00");
    expect(formatCountdown(5)).toBe("00:00:05");
    expect(formatCountdown(3661)).toBe("01:01:01");
    expect(formatCountdown(90061)).toBe("1天 01:01:01");
  });

  it("uses the synced server offset, not the local clock", () => {
    // Pretend the server is 1000s ahead of this machine.
    _setOffsetForTest(1000);
    const localSec = Math.floor(Date.now() / 1000);
    expect(serverNow()).toBeGreaterThanOrEqual(localSec + 999);

    // A sale 60 server-seconds out reads ~60, not skewed by the local clock.
    const target = serverNow() + 60;
    expect(secondsUntil(target)).toBeGreaterThanOrEqual(59);
    expect(secondsUntil(target)).toBeLessThanOrEqual(61);

    expect(secondsUntil(serverNow() - 10)).toBe(0); // past → floored at 0
    _setOffsetForTest(0);
  });

  it("ratchets the server clock forward, ignoring out-of-order older samples", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-06-20T11:00:00Z"));
    _setOffsetForTest(0);
    const local = Math.floor(Date.now() / 1000);

    syncServerClock(local + 100); // server +100s ahead
    expect(clockOffset()).toBe(100);
    syncServerClock(local + 70); // an older, out-of-order node poll → must NOT retreat
    expect(clockOffset()).toBe(100);
    syncServerClock(local + 130); // a fresher poll → ratchet forward
    expect(clockOffset()).toBe(130);
    syncServerClock(local - 1000); // huge downward jump → genuine desync, accept
    expect(clockOffset()).toBe(-1000);

    _setOffsetForTest(0);
    vi.useRealTimers();
  });

  it("suppresses a duplicated screen date when the name already contains it", () => {
    const date = formatDate(1783641600); // 2026-07-10 UTC
    expect(date).toBe("2026-07-10 周五");
    expect(shouldShowDateBesideName("2026-07-10 周五", date)).toBe(false);
    expect(shouldShowDateBesideName("2026-07-10 周五 19:30", date)).toBe(false);
    expect(shouldShowDateBesideName("晚场", date)).toBe(true);
  });

  it("formats relative age from the synced server clock", () => {
    _setOffsetForTest(1000);
    const now = serverNow();
    expect(formatAge(now - 20)).toBe("20s 前");
    expect(formatAge(now - 125)).toBe("2m 前");
    expect(formatAge(now - 7200)).toBe("2h 前");
    _setOffsetForTest(0);
  });
});
