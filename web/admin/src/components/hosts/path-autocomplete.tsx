import { useState, useRef, useEffect, useCallback } from "react";
import { Loader2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { useHostFiles } from "@/hooks/use-host-files";

interface PathAutocompleteProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

function getQueryAndFilter(value: string): { queryPath: string; filter: string } {
  if (value.endsWith("/")) {
    return { queryPath: value.slice(0, -1) || "/", filter: "" };
  }
  const lastSlash = value.lastIndexOf("/");
  if (lastSlash === -1) return { queryPath: "/", filter: value };
  return {
    queryPath: value.slice(0, lastSlash) || "/",
    filter: value.slice(lastSlash + 1),
  };
}

export function PathAutocomplete({
  value,
  onChange,
  placeholder,
  disabled,
  className,
}: PathAutocompleteProps) {
  const [open, setOpen] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(0);
  const containerRef = useRef<HTMLDivElement>(null);
  const blurTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const { queryPath, filter } = getQueryAndFilter(value);
  const { data, isLoading } = useHostFiles(queryPath);
  const allEntries = data?.entries ?? [];
  const entries = filter
    ? allEntries.filter((e) =>
        e.toLowerCase().startsWith(filter.toLowerCase()),
      )
    : allEntries;

  const showDropdown = open && value.startsWith("/");

  const handleFocus = () => {
    if (value.startsWith("/")) {
      setOpen(true);
    }
  };

  const handleBlur = () => {
    blurTimeoutRef.current = setTimeout(() => {
      setOpen(false);
    }, 150);
  };

  const handleSelect = useCallback(
    (entry: string) => {
      const { queryPath: qp } = getQueryAndFilter(value);
      onChange(qp === "/" ? "/" + entry : qp + "/" + entry);
      setOpen(false);
    },
    [value, onChange],
  );

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (!showDropdown || entries.length === 0) return;

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setHighlightedIndex((i) => (i + 1) % entries.length);
        break;
      case "ArrowUp":
        e.preventDefault();
        setHighlightedIndex((i) => (i - 1 + entries.length) % entries.length);
        break;
      case "Enter":
        e.preventDefault();
        if (entries[highlightedIndex]) {
          handleSelect(entries[highlightedIndex]);
        }
        break;
      case "Escape":
        e.preventDefault();
        setOpen(false);
        break;
    }
  };

  useEffect(() => {
    setHighlightedIndex(0);
  }, [entries.length]);

  useEffect(() => {
    return () => {
      if (blurTimeoutRef.current) {
        clearTimeout(blurTimeoutRef.current);
      }
    };
  }, []);

  return (
    <div ref={containerRef} className="relative">
      <Input
        value={value}
        onChange={(e) => {
          onChange(e.target.value);
          if (e.target.value.startsWith("/")) {
            setOpen(true);
          }
        }}
        onFocus={handleFocus}
        onBlur={handleBlur}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        disabled={disabled}
        className={className}
      />
      {isLoading && (
        <div className="absolute right-2 top-1/2 -translate-y-1/2">
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        </div>
      )}
      {showDropdown && (
        <div className="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-md border bg-popover shadow-md">
          {entries.length === 0 ? (
            <div className="px-3 py-2 text-sm text-muted-foreground">
              {isLoading ? "加载中..." : "无可用子目录"}
            </div>
          ) : (
            <ul className="py-1">
              {entries.map((entry, i) => (
                <li
                  key={entry}
                  className={`cursor-pointer px-3 py-1.5 text-sm truncate ${
                    i === highlightedIndex
                      ? "bg-accent text-accent-foreground"
                      : ""
                  }`}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    handleSelect(entry);
                  }}
                  onMouseEnter={() => setHighlightedIndex(i)}
                >
                  {entry}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
