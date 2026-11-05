import { expect, it } from "vitest";
import { createSseParser, type SseMessage } from "./sse";

it("assembles events split across chunks and skips heartbeats", () => {
  const messages: SseMessage[] = [];
  const parser = createSseParser((message) => messages.push(message));
  parser.push(": connected\n\nid: c1\nev");
  parser.push('ent: notification\ndata: {"id"');
  parser.push(':"n1"}\n');
  expect(messages).toHaveLength(0);
  parser.push("\n: ping\n\n");
  parser.push('event: unread-count\r\ndata: {"count":2}\r\n\r\n');

  expect(messages).toEqual([
    { id: "c1", event: "notification", data: '{"id":"n1"}' },
    // id carries over to later events, as in the SSE spec
    { id: "c1", event: "unread-count", data: '{"count":2}' },
  ]);
  expect(parser.lastEventId).toBe("c1");
});

it("joins multi-line data", () => {
  const messages: SseMessage[] = [];
  const parser = createSseParser((message) => messages.push(message));
  parser.push("data: a\ndata: b\n\n");
  expect(messages[0]).toEqual({ id: "", event: "message", data: "a\nb" });
});

it("does not split an event when CRLF is cut between chunks", () => {
  const messages: SseMessage[] = [];
  const parser = createSseParser((message) => messages.push(message));
  parser.push("data: a\r");
  parser.push("\ndata: b\r\n\r\n");
  expect(messages).toEqual([{ id: "", event: "message", data: "a\nb" }]);
});
