import * as React from "react";
import { Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

interface SearchInputProps
  extends Omit<React.ComponentProps<"input">, "type"> {
  // `containerClassName` controls the outer relative wrapper (width,
  // responsive sizing). The Tags/Categories list pages use `w-full max-w-sm`;
  // API-keys uses `w-full max-w-xs`; the Transactions toolbar uses
  // `w-full min-w-48 sm:w-64`. Default is `w-full max-w-sm` (matches the
  // most common list-page pattern).
  containerClassName?: string;
}

// SearchInput is the canonical text input with a leading magnifier glyph
// used by every v2 list page (Tags, Categories, API keys, Transactions).
// Geometry: `Search` is a `size-4 text-muted-foreground` absolutely
// positioned at `top-1/2 left-2.5 -translate-y-1/2`; the input is the
// stock `<Input>` primitive with `pl-8` to clear the icon.
//
// The icon is `pointer-events-none` so clicking on it focuses the input
// underneath instead of swallowing the event. Forward all native input
// props (value, onChange, onKeyDown, placeholder, etc.) plus a ref via
// React's standard ref forwarding.
//
// Don't fork the look — extend this primitive.
export const SearchInput = React.forwardRef<HTMLInputElement, SearchInputProps>(
  function SearchInput(
    { containerClassName, className, ...props },
    ref,
  ) {
    return (
      <div className={cn("relative w-full max-w-sm", containerClassName)}>
        <Search
          className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
          aria-hidden
        />
        <Input
          ref={ref}
          type="search"
          // iOS form-ergonomics defaults — every consumer of SearchInput
          // gets the search-tinted keyboard (magnifying-glass return key)
          // for free, with no autocapitalize/autocorrect interfering with
          // merchant slugs, IDs, and other technical query terms. Consumers
          // can still override any of these via props.
          inputMode="search"
          enterKeyHint="search"
          autoComplete="off"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          className={cn("pl-8", className)}
          {...props}
        />
      </div>
    );
  },
);
