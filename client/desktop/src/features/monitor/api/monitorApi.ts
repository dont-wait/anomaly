import { API_BASE_URL, readAdminSession } from "@/features/auth/api/adminSession";
import type {
  MonitorDashboardResponse,
  RiskAssessmentEvent,
  TimeRange,
} from "../model";

export class MonitorApiError extends Error {
  readonly status?: number;

  constructor(message = "Không thể tải dữ liệu giám sát rủi ro", status?: number) {
    super(message);
    this.name = "MonitorApiError";
    this.status = status;
  }
}

function authHeaders(): HeadersInit {
  const session = readAdminSession();
  return session ? { Authorization: `Bearer ${session.accessToken}` } : {};
}

async function fetchWithTimeout(input: RequestInfo | URL, init: RequestInit = {}, timeoutMs = 8000): Promise<Response> {
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
  const response = await fetchWithTimeout(`${API_BASE_URL}/api/admin/monitor?range=${range}`, {
    headers: authHeaders(),
    signal,
  });
  if (!response.ok) {
    throw new MonitorApiError("Không thể tải dữ liệu giám sát rủi ro", response.status);
  }
  return (await response.json()) as MonitorDashboardResponse;
}

function parseSseBlock(block: string): RiskAssessmentEvent | null {
  let eventName = "message";
  let data = "";
  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) eventName = line.slice(6).trim();
    if (line.startsWith("data:")) data += line.slice(5).trim();
  }
  if (eventName !== "risk_assessment.created" || !data) return null;
  return JSON.parse(data) as RiskAssessmentEvent;
}

export async function consumeRiskStream(
  cursor: string,
  onEvent: (event: RiskAssessmentEvent, cursor: string) => void,
  signal: AbortSignal,
): Promise<void> {
  const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : "";
  const response = await fetch(`${API_BASE_URL}/api/admin/monitor/stream${query}`, {
    headers: { ...authHeaders(), Accept: "text/event-stream" },
    signal,
  });
  if (!response.ok) {
    throw new MonitorApiError("Không thể kết nối luồng realtime", response.status);
  }
  if (!response.body) throw new MonitorApiError("Luồng realtime không có dữ liệu");

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let eventCursor = "";
  try {
    while (!signal.aborted) {
      const chunk = await reader.read();
      if (chunk.done) break;
      buffer += decoder.decode(chunk.value, { stream: true }).replace(/\r\n/g, "\n");
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
