import {
  API_BASE_URL,
  readAdminSession,
} from "@/features/auth/api/adminSession";
import type {
  MonitorDashboardResponse,
  RiskAssessmentEvent,
  RiskStreamEvent,
  TransactionStatusEvent,
  TimeRange,
} from "../model";

export class MonitorApiError extends Error {
  readonly status?: number;

  constructor(
    message = "Không thể tải dữ liệu giám sát rủi ro",
    status?: number,
  ) {
    super(message);
    this.name = "MonitorApiError";
    this.status = status;
  }
}

function cursorSequence(cursor: string): bigint | null {
  if (!cursor) return null;
  try {
    const normalized = cursor
      .replace(/-/g, "+")
      .replace(/_/g, "/")
      .padEnd(Math.ceil(cursor.length / 4) * 4, "=");
    const bytes = atob(normalized);
    const payload = JSON.parse(bytes) as { sequence?: unknown };
    if (
      typeof payload.sequence !== "number" ||
      !Number.isSafeInteger(payload.sequence) ||
      payload.sequence <= 0
    )
      return null;
    return BigInt(payload.sequence);
  } catch {
    return null;
  }
}

export function shouldAdvanceMonitorCursor(
  current: string,
  next: string,
): boolean {
  if (!next) return false;
  if (!current) return true;
  const currentSequence = cursorSequence(current);
  const nextSequence = cursorSequence(next);
  if (currentSequence !== null && nextSequence !== null)
    return nextSequence >= currentSequence;
  return false;
}

function authHeaders(): HeadersInit {
  const session = readAdminSession();
  return session ? { Authorization: `Bearer ${session.accessToken}` } : {};
}

async function fetchWithTimeout(
  input: RequestInfo | URL,
  init: RequestInit = {},
  timeoutMs = 8000,
): Promise<Response> {
  const controller = new AbortController();
  const externalSignal = init.signal;
  const abort = () => controller.abort();
  if (externalSignal) {
    if (externalSignal.aborted) controller.abort();
    else externalSignal.addEventListener("abort", abort, { once: true });
  }
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(input, { ...init, signal: controller.signal });
  } finally {
    window.clearTimeout(timer);
    externalSignal?.removeEventListener("abort", abort);
  }
}

export async function getMonitorDashboard(
  range: TimeRange,
  signal?: AbortSignal,
): Promise<MonitorDashboardResponse> {
  const response = await fetchWithTimeout(
    `${API_BASE_URL}/api/admin/monitor?range=${range}`,
    {
      headers: authHeaders(),
      signal,
    },
  );
  if (!response.ok) {
    throw new MonitorApiError(
      "Không thể tải dữ liệu giám sát rủi ro",
      response.status,
    );
  }
  return (await response.json()) as MonitorDashboardResponse;
}

function parseSseBlock(block: string): RiskStreamEvent | null {
  let eventName = "message";
  let data = "";
  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) eventName = line.slice(6).trim();
    if (line.startsWith("data:")) data += line.slice(5).trim();
  }
  if (!data) return null;
  if (eventName === "risk_assessment.created")
    return {
      kind: "assessment",
      event: JSON.parse(data) as RiskAssessmentEvent,
    };
  if (eventName === "transaction.status_changed")
    return {
      kind: "transaction_status",
      event: JSON.parse(data) as TransactionStatusEvent,
    };
  return null;
}

export async function consumeRiskStream(
  cursor: string,
  onEvent: (event: RiskStreamEvent, cursor: string) => void,
  signal: AbortSignal,
  onConnected?: () => void,
): Promise<void> {
  const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : "";
  const response = await fetch(
    `${API_BASE_URL}/api/admin/monitor/stream${query}`,
    {
      headers: { ...authHeaders(), Accept: "text/event-stream" },
      signal,
    },
  );
  if (!response.ok) {
    throw new MonitorApiError(
      "Không thể kết nối luồng realtime",
      response.status,
    );
  }
  if (!response.body)
    throw new MonitorApiError("Luồng realtime không có dữ liệu");
  onConnected?.();

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let eventCursor = "";
  try {
    while (!signal.aborted) {
      const chunk = await reader.read();
      if (chunk.done) break;
      buffer += decoder
        .decode(chunk.value, { stream: true })
        .replace(/\r\n/g, "\n");
      let boundary = buffer.indexOf("\n\n");
      while (boundary >= 0) {
        const block = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);
        for (const line of block.split("\n")) {
          if (line.startsWith("id:")) eventCursor = line.slice(3).trim();
        }
        const event = parseSseBlock(block);
        if (event) onEvent(event, eventCursor);
        boundary = buffer.indexOf("\n\n");
      }
    }
  } finally {
    reader.releaseLock();
  }
}
