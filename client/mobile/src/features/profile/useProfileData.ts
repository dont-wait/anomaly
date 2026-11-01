import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchProfileMedia,
  getDocumentSlots,
} from "@/features/auth/api/profileMutations";
import type { ProfileDocumentSlot } from "@/features/auth/api/profile";
import { toProfileError } from "./profileHelpers";

export interface ProfileDocumentsState {
  slots: ProfileDocumentSlot[];
  loading: boolean;
  error: string;
  reload: () => Promise<void>;
}

export function useProfileDocuments(
  open: boolean,
  token: string | null,
): ProfileDocumentsState {
  const [slots, setSlots] = useState<ProfileDocumentSlot[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const requestRef = useRef(0);

  const reload = useCallback(async () => {
    const request = ++requestRef.current;
    if (!token) return;
    setLoading(true);
    setError("");
    try {
      const next = await getDocumentSlots(token);
      if (request === requestRef.current) setSlots(next);
    } catch (cause) {
      if (request === requestRef.current) setError(toProfileError(cause));
    } finally {
      if (request === requestRef.current) setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- tải dữ liệu khi mở hồ sơ
    if (open) void reload();
  }, [open, reload]);

  return { slots, loading, error, reload };
}

/** Thẻ img không gửi được bearer token, nên tải ảnh thành blob qua backend. */
export function useProfileImage(token: string | null, mediaId?: string) {
  const [url, setUrl] = useState<string>();

  useEffect(() => {
    if (!token || !mediaId) return;
    const controller = new AbortController();
    let objectUrl: string | undefined;
    fetchProfileMedia(token, mediaId, { signal: controller.signal })
      .then((blob) => {
        if (controller.signal.aborted) return;
        objectUrl = URL.createObjectURL(blob);
        setUrl(objectUrl);
      })
      .catch(() => {
        // Không tải được thì Avatar hiển thị chữ cái đầu tên.
      });
    return () => {
      controller.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
      setUrl(undefined);
    };
  }, [mediaId, token]);

  return url;
}

/** Object URL xem trước tệp người dùng vừa chọn. */
export function useFilePreview(file: Blob | null) {
  const [url, setUrl] = useState<string>();

  useEffect(() => {
    if (!file) return;
    const objectUrl = URL.createObjectURL(file);
    // eslint-disable-next-line react-hooks/set-state-in-effect -- đồng bộ với object URL bên ngoài React
    setUrl(objectUrl);
    return () => {
      URL.revokeObjectURL(objectUrl);
      setUrl(undefined);
    };
  }, [file]);

  return url;
}
