import { useEffect, useState } from "react";

// Returns a debounced copy of `value` that only updates after `delayMs` of
// no further changes. Used to keep URL search params (and the queries keyed
// on them) from churning on every keystroke.
export function useDebouncedValue<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(id);
  }, [value, delayMs]);

  return debounced;
}
