export interface SseMessage {
  /** Giá trị `id:` gần nhất (SSE giữ lại id qua các event) */
  id: string;
  event: string;
  data: string;
}

/**
 * Parser SSE tăng dần: nhận từng chunk text (có thể cắt giữa dòng) và gọi
 * `onMessage` cho mỗi event hoàn chỉnh. Dòng comment (`: ping`) bị bỏ qua.
 */
export function createSseParser(onMessage: (message: SseMessage) => void) {
  let buffer = "";
  let lastId = "";

  const dispatch = (block: string) => {
    let event = "message";
    const data: string[] = [];
    let hasField = false;
    for (const line of block.split("\n")) {
      if (line === "" || line.startsWith(":")) continue;
      const colon = line.indexOf(":");
      const field = colon < 0 ? line : line.slice(0, colon);
      let value = colon < 0 ? "" : line.slice(colon + 1);
      if (value.startsWith(" ")) value = value.slice(1);
      hasField = true;
      if (field === "id") lastId = value;
      else if (field === "event") event = value;
      else if (field === "data") data.push(value);
    }
    if (hasField && data.length > 0) {
      onMessage({ id: lastId, event, data: data.join("\n") });
    }
  };

  return {
    push(chunk: string) {
      buffer += chunk;
      // A trailing "\r" may be the first half of "\r\n": wait for the next chunk.
      const pendingCR = buffer.endsWith("\r");
      buffer = (pendingCR ? buffer.slice(0, -1) : buffer).replace(
        /\r\n?/g,
        "\n",
      );
      let boundary = buffer.indexOf("\n\n");
      while (boundary >= 0) {
        dispatch(buffer.slice(0, boundary));
        buffer = buffer.slice(boundary + 2);
        boundary = buffer.indexOf("\n\n");
      }
      if (pendingCR) buffer += "\r";
    },
    get lastEventId() {
      return lastId;
    },
  };
}
