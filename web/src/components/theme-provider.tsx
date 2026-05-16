import * as React from "react";
import { ThemeProvider as NextThemesProvider } from "next-themes";

// Wraps `next-themes` so the SPA can flip between light / dark / system.
// Mounted at the React root (in `main.tsx`) so every consumer — including
// `ui/sonner.tsx`, which already reads `useTheme()` — gets the right
// foreground/background tokens. The provider toggles a `.dark` class on
// `<html>`, which globals.css's `:is(.dark *)` custom-variant already
// keys off of, so no extra CSS work is needed.
//
// `attribute="class"` matches Tailwind v4's recommended setup with
// `@custom-variant dark (&:is(.dark *))` in globals.css.
//
// `disableTransitionOnChange` avoids the brief flicker of tokens
// transitioning when the user toggles theme — a calmer switch reads
// as intentional rather than a CSS regression.
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme="system"
      enableSystem
      disableTransitionOnChange
      storageKey="breadbox:theme"
    >
      {children}
    </NextThemesProvider>
  );
}
