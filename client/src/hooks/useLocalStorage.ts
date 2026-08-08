import { useState, useEffect } from "react";

export function useLocalStorage<T>(key: string, initialValue: T) {
  const [loadedKey, setLoadedKey] = useState(key);
  const [value, setValue] = useState<T>(() => {
    try {
      const item = window.localStorage.getItem(key);
      return item ? (JSON.parse(item) as T) : initialValue;
    } catch {
      return initialValue;
    }
  });

  // Rehydrate during render when key changes (avoids setState-in-effect)
  if (key !== loadedKey) {
    setLoadedKey(key);
    try {
      const item = window.localStorage.getItem(key);
      setValue(item ? (JSON.parse(item) as T) : initialValue);
    } catch {
      setValue(initialValue);
    }
  }

  // Persist only for the key that was last loaded
  useEffect(() => {
    if (key !== loadedKey) return;
    try {
      window.localStorage.setItem(key, JSON.stringify(value));
    } catch {
      // Serialization or storage failed; value stays in memory only
    }
  }, [key, loadedKey, value]);

  return [value, setValue] as const;
}
