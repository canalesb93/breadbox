import { useEffect, useRef } from "react";

export interface Shortcut {
  id: string;
  keys: string[];
  label: string;
  group?: string;
}

const registry = new Map<string, Shortcut>();
const listeners = new Set<() => void>();

function notify() {
  listeners.forEach((fn) => fn());
}

export function registerShortcut(shortcut: Shortcut): () => void {
  registry.set(shortcut.id, shortcut);
  notify();
  return () => {
    registry.delete(shortcut.id);
    notify();
  };
}

export function listShortcuts(): Shortcut[] {
  return Array.from(registry.values());
}

export function subscribeShortcuts(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

interface KeyMatch {
  key: string;
  meta?: boolean;
  shift?: boolean;
  alt?: boolean;
  ctrl?: boolean;
}

function parseKeys(keys: string[]): KeyMatch {
  const match: KeyMatch = { key: "" };
  for (const raw of keys) {
    const k = raw.toLowerCase();
    if (k === "mod" || k === "cmd" || k === "⌘") match.meta = true;
    else if (k === "ctrl") match.ctrl = true;
    else if (k === "shift" || k === "⇧") match.shift = true;
    else if (k === "alt" || k === "option" || k === "⌥") match.alt = true;
    else match.key = k;
  }
  return match;
}

export interface UseShortcutOptions {
  label: string;
  group?: string;
  enabled?: boolean;
}

export function useShortcut(
  keys: string[],
  handler: (event: KeyboardEvent) => void,
  options: UseShortcutOptions,
): void {
  const { label, group, enabled = true } = options;
  const id = `${group ?? "global"}:${label}`;
  // Both `keys` (inline array literal) and `handler` (inline arrow) are
  // referentially unstable across renders. Without this, the effect tears
  // down and re-adds the global keydown listener — and churns the registry —
  // on every render of the calling component. Depend on a stable string for
  // keys, and read the handler through a ref.
  const keysKey = keys.join("+");
  const handlerRef = useRef(handler);
  handlerRef.current = handler;

  useEffect(() => {
    if (!enabled) return;
    const parsedKeys = keysKey.split("+");
    const unregister = registerShortcut({ id, keys: parsedKeys, label, group });
    const match = parseKeys(parsedKeys);
    const onKey = (event: KeyboardEvent) => {
      if (event.key.toLowerCase() !== match.key) return;
      if (!!match.meta !== (event.metaKey || event.ctrlKey)) return;
      if (!!match.shift !== event.shiftKey) return;
      if (!!match.alt !== event.altKey) return;
      const target = event.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.isContentEditable)
      ) {
        return;
      }
      handlerRef.current(event);
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      unregister();
    };
  }, [enabled, id, keysKey, label, group]);
}
