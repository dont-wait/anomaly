import { useEffect, useRef, useState } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { DocumentStep } from "./DocumentStep";

const readers: Reader[] = [];
class Reader {
  result: ArrayBuffer | null = null;
  error = new Error("Temporary capture file unavailable");
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  readAsArrayBuffer() {
    readers.push(this);
  }
  finish(bytes: number[]) {
    this.result = new Uint8Array(bytes).buffer;
    this.onload?.();
  }
}
let stored: { front: File | null; back: File | null };
function Harness() {
  const [documents, setDocuments] = useState({
    front: null as File | null,
    back: null as File | null,
  });
  const [error, setError] = useState("");
  useEffect(() => {
    stored = documents;
  }, [documents]);
  return (
    <DocumentStep
      documents={documents}
      setDocuments={setDocuments}
      error={error}
      setError={setError}
      heading={useRef(null)}
      submit={vi.fn()}
    />
  );
}
beforeEach(() => {
  readers.length = 0;
  vi.stubGlobal("FileReader", Reader);
  URL.createObjectURL = vi.fn(() => "blob:document-snapshot");
  URL.revokeObjectURL = vi.fn();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const select = (side: "trước" | "sau", file: File) =>
  fireEvent.change(screen.getByLabelText(`Ảnh mặt ${side} CCCD`), {
    target: { files: [file] },
  });
const nextDisabled = () =>
  (screen.getByRole("button", { name: "Tiếp tục" }) as HTMLButtonElement)
    .disabled;

it("keeps detached in-memory files and blocks navigation until both reads finish", async () => {
  render(<Harness />);
  const front = new File(["temporary"], "front.jpg", {
    type: "image/jpeg",
    lastModified: 123,
  });
  const back = new File(["temporary"], "back.png", { type: "image/png" });
  select("trước", front);
  select("sau", back);
  expect(stored.front).toBeNull();
  expect(nextDisabled()).toBe(true);
  await act(async () => readers[0].finish([1, 2, 3]));
  expect(nextDisabled()).toBe(true);
  await act(async () => readers[1].finish([4, 5]));
  expect(nextDisabled()).toBe(false);
  expect(stored.front).not.toBe(front);
  expect(stored.front?.name).toBe("front.jpg");
  expect(stored.front?.type).toBe("image/jpeg");
  expect(stored.front?.lastModified).toBe(123);
  expect(stored.front?.size).toBe(3);
  expect(stored.back?.size).toBe(2);
});
it("does not overwrite a newer selection when an older read finishes last", async () => {
  render(<Harness />);
  select("trước", new File(["old"], "old.jpg", { type: "image/jpeg" }));
  select("trước", new File(["new"], "new.jpg", { type: "image/jpeg" }));
  await act(async () => readers[1].finish([2]));
  await act(async () => readers[0].finish([1]));
  expect(stored.front?.name).toBe("new.jpg");
});
it("asks for a new image if Android cannot read its temporary file", async () => {
  render(<Harness />);
  select("trước", new File(["temp"], "front.jpg", { type: "image/jpeg" }));
  await act(async () => readers[0].onerror?.());
  expect(stored.front).toBeNull();
  expect(screen.getByRole("alert").textContent).toContain(
    "Không đọc được ảnh CCCD",
  );
  expect(nextDisabled()).toBe(true);
});
