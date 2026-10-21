import { beforeEach, describe, expect, it } from "vitest";

import { getDeviceId, getPlatform } from "./device";

describe("device metadata", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("persists one device ID across calls", () => {
    const first = getDeviceId();
    const second = getDeviceId();

    expect(first).toBeTruthy();
    expect(second).toBe(first);
    expect(window.localStorage.getItem("anomaly.deviceId")).toBe(first);
  });

  it("uses the stored device ID when one exists", () => {
    window.localStorage.setItem("anomaly.deviceId", "DEVICE01");

    expect(getDeviceId()).toBe("DEVICE01");
  });

  it.each([
    ["Mozilla/5.0 Android", "ANDROID"],
    ["Mozilla/5.0 iPhone", "IOS"],
    ["Mozilla/5.0 Windows NT 10.0", "DESKTOP"],
    ["Mozilla/5.0 X11; Linux x86_64", "DESKTOP"],
  ])("detects %s as %s", (userAgent, expected) => {
    Object.defineProperty(window.navigator, "userAgent", {
      configurable: true,
      value: userAgent,
    });

    expect(getPlatform()).toBe(expected);
  });
});
