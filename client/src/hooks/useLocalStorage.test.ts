import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useLocalStorage } from "./useLocalStorage";

describe("useLocalStorage", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("should return initial value when localStorage is empty", () => {
    const { result } = renderHook(() => useLocalStorage("test-key", "default"));
    expect(result.current[0]).toBe("default");
  });

  it("should persist value to localStorage", () => {
    const { result } = renderHook(() => useLocalStorage("test-key", "default"));

    act(() => {
      result.current[1]("updated");
    });

    expect(result.current[0]).toBe("updated");
    expect(localStorage.getItem("test-key")).toBe('"updated"');
  });

  it("should handle serialization errors without throwing", () => {
    const { result } = renderHook(() => useLocalStorage("test-key", "default"));

    const circular: Record<string, unknown> = {};
    circular.self = circular;

    act(() => {
      result.current[1](circular as unknown as string);
    });

    // Value stays in memory, no crash
    expect(result.current[0]).toBe(circular);
    // localStorage remains unchanged (no persisted value)
    expect(localStorage.getItem("test-key")).toBeNull();
  });

  it("should handle localStorage.setItem errors without throwing", () => {
    const { result } = renderHook(() => useLocalStorage("test-key", "default"));

    const error = new Error("Quota exceeded");
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw error;
    });

    act(() => {
      result.current[1]("new value");
    });

    // Value stays in memory, no crash
    expect(result.current[0]).toBe("new value");
  });
});
