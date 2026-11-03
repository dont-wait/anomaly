import { useCallback, useEffect, useState } from "react";
import {
  deleteTermsDraft,
  explainProductError,
  listTerms,
  publishTerms,
  saveTermsDraft,
} from "./api";
import type { TermsDocument, TermsSection } from "./model";
import { fieldInput, fieldLabel, panel } from "./format";
import { ActionButton, Banner } from "./ui";

const STATUS_LABELS: Record<string, string> = {
  DRAFT: "Bản nháp",
  ACTIVE: "Đang áp dụng",
  INACTIVE: "Đã thay thế",
};

export function SavingsTermsPanel() {
  const [items, setItems] = useState<TermsDocument[]>([]);
  const [title, setTitle] = useState("");
  const [introduction, setIntroduction] = useState("");
  const [sections, setSections] = useState<TermsSection[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const fill = useCallback((loaded: TermsDocument[]) => {
    setItems(loaded);
    // Sửa từ bản nháp nếu có, không thì từ bản đang áp dụng.
    const base =
      loaded.find((item) => item.status === "DRAFT") ??
      loaded.find((item) => item.status === "ACTIVE");
    setTitle(base?.title ?? "");
    setIntroduction(base?.introduction ?? "");
    setSections(base?.sections ?? []);
  }, []);

  useEffect(() => {
    let active = true;
    listTerms()
      .then((loaded) => {
        if (active) fill(loaded);
      })
      .catch((failure: unknown) => {
        if (active) setError(explainProductError(failure));
      });
    return () => {
      active = false;
    };
  }, [fill]);

  const run = async (action: () => Promise<unknown>, success: string) => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action();
      setMessage(success);
      fill(await listTerms());
    } catch (failure) {
      setError(explainProductError(failure));
    } finally {
      setBusy(false);
    }
  };

  const draft = items.find((item) => item.status === "DRAFT");
  const setSection = (index: number, change: Partial<TermsSection>) =>
    setSections((old) =>
      old.map((section, i) =>
        i === index ? { ...section, ...change } : section,
      ),
    );
  const fingerprint = (t: string, intro: string, list: TermsSection[]) =>
    JSON.stringify([t, intro, list.map((item) => [item.title, item.body])]);
  const unsaved =
    Boolean(draft) &&
    fingerprint(title, introduction, sections) !==
      fingerprint(
        draft?.title ?? "",
        draft?.introduction ?? "",
        draft?.sections ?? [],
      );
  const valid =
    title.trim() !== "" &&
    introduction.trim() !== "" &&
    sections.length > 0 &&
    sections.every((s) => s.title.trim() !== "" && s.body.trim() !== "");

  return (
    <section className={`${panel} grid gap-3 p-4`}>
      {error && <Banner message={error} error />}
      {message && <Banner message={message} />}
      <p className="text-[11px] text-(--admin-text-muted)">
        Khách mở sổ phải đồng ý đúng bản đang áp dụng. Khi đổi kỳ hạn hoặc mức
        gửi tối thiểu, hãy rà soát văn bản vì có thể đang nêu các con số này.
      </p>
      <label className={fieldLabel}>
        Tiêu đề
        <input
          className={fieldInput}
          value={title}
          onChange={(event) => setTitle(event.target.value)}
        />
      </label>
      <label className={fieldLabel}>
        Giới thiệu
        <textarea
          className={`${fieldInput} h-20 py-2`}
          value={introduction}
          onChange={(event) => setIntroduction(event.target.value)}
        />
      </label>
      {sections.map((section, index) => (
        <div
          key={index}
          className="grid gap-2 rounded-sm border border-(--admin-border) p-3"
        >
          <label className={fieldLabel}>
            Tiêu đề mục {index + 1}
            <input
              className={fieldInput}
              value={section.title}
              onChange={(event) =>
                setSection(index, { title: event.target.value })
              }
            />
          </label>
          <label className={fieldLabel}>
            Nội dung mục {index + 1}
            <textarea
              className={`${fieldInput} h-24 py-2`}
              value={section.body}
              onChange={(event) =>
                setSection(index, { body: event.target.value })
              }
            />
          </label>
          <div>
            <ActionButton
              variant="secondary"
              onClick={() =>
                setSections((old) => old.filter((_, i) => i !== index))
              }
            >
              Xóa mục {index + 1}
            </ActionButton>
          </div>
        </div>
      ))}
      {unsaved && (
        <Banner message="Có thay đổi chưa lưu. Hãy lưu bản nháp trước khi phát hành." />
      )}
      <div className="flex flex-wrap gap-2">
        <ActionButton
          variant="secondary"
          onClick={() =>
            setSections((old) => [...old, { title: "", body: "" }])
          }
        >
          Thêm mục
        </ActionButton>
        <ActionButton
          variant="secondary"
          disabled={busy || !valid}
          onClick={() =>
            void run(
              () =>
                saveTermsDraft({
                  version: "",
                  status: "DRAFT",
                  title,
                  introduction,
                  sections,
                }),
              "Đã lưu bản nháp điều khoản.",
            )
          }
        >
          Lưu bản nháp
        </ActionButton>
        {draft && (
          <>
            <ActionButton
              disabled={busy || unsaved}
              onClick={() => {
                if (window.confirm("Phát hành điều khoản này?"))
                  void run(
                    () => publishTerms(draft.version),
                    "Đã phát hành điều khoản.",
                  );
              }}
            >
              Phát hành
            </ActionButton>
            <ActionButton
              variant="secondary"
              disabled={busy}
              onClick={() => {
                if (window.confirm("Xóa bản nháp điều khoản?"))
                  void run(deleteTermsDraft, "Đã xóa bản nháp.");
              }}
            >
              Xóa bản nháp
            </ActionButton>
          </>
        )}
      </div>
      <ul className="grid gap-1 border-t border-(--admin-border) pt-3 text-[11px] text-(--admin-text-muted)">
        {items.map((item) => (
          <li key={item.version}>
            {item.version} · {STATUS_LABELS[item.status] ?? item.status}
          </li>
        ))}
      </ul>
    </section>
  );
}
