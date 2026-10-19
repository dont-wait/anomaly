const DEVICE_ID_STORAGE_KEY = "anomaly.deviceId";

function createDeviceId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

export function getDeviceId(): string {
  if (typeof window === "undefined") return "UNKNOWN";

  try {
    const existing = window.localStorage.getItem(DEVICE_ID_STORAGE_KEY)?.trim();
    if (existing) return existing;

    const generated = createDeviceId();
    window.localStorage.setItem(DEVICE_ID_STORAGE_KEY, generated);
    return generated;
  } catch {
    return "UNKNOWN";
  }
}

export function getPlatform(): string {
  if (typeof navigator === "undefined") return "UNKNOWN";

  const userAgent = navigator.userAgent.toLowerCase();
  if (userAgent.includes("android")) return "ANDROID";
  if (/iphone|ipad|ipod/.test(userAgent)) return "IOS";
  if (/windows|macintosh|linux/.test(userAgent)) return "DESKTOP";
  return "WEB";
}
