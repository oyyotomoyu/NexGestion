import { useEffect, useRef, useState } from "react";

import { NexText } from "@/components/NexText";

import "./style.css";

export interface NexSelectOption<Value extends string> {
  value: Value;
  label: string;
}

interface NexSelectProps<Value extends string> {
  ariaLabel: string;
  value: Value;
  options: NexSelectOption<Value>[];
  onChange: (value: Value) => void;
  menuPlacement?: "bottom" | "top";
}

export function NexSelect<Value extends string>({
  ariaLabel,
  menuPlacement = "bottom",
  onChange,
  options,
  value,
}: NexSelectProps<Value>) {
  const [isOpen, setIsOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const selectedOption = options.find((option) => option.value === value) ?? options[0];

  useEffect(() => {
    function handleOutsideClick(event: PointerEvent) {
      if (!rootRef.current?.contains(event.target as Node)) {
        setIsOpen(false);
      }
    }

    document.addEventListener("pointerdown", handleOutsideClick);
    return () => document.removeEventListener("pointerdown", handleOutsideClick);
  }, []);

  return (
    <div className="nex-select" data-menu-placement={menuPlacement} ref={rootRef}>
      <button
        className="nex-select__trigger"
        type="button"
        aria-label={ariaLabel}
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        disabled={options.length === 0}
        onClick={() => setIsOpen((open) => !open)}
        onKeyDown={(event) => {
          if (event.key === "Escape") setIsOpen(false);
          if (event.key === "ArrowDown") setIsOpen(true);
        }}
      >
        <NexText as="span" variant="label" color="inherit">
          {selectedOption?.label ?? ""}
        </NexText>
        <span className="nex-select__chevron" aria-hidden="true" />
      </button>

      {isOpen && (
        <div className="nex-select__menu" role="listbox" aria-label={ariaLabel}>
          {options.map((option) => (
            <button
              className={`nex-select__option${option.value === value ? " active" : ""}`}
              key={option.value}
              type="button"
              role="option"
              aria-selected={option.value === value}
              onClick={() => {
                onChange(option.value);
                setIsOpen(false);
              }}
            >
              <NexText as="span" variant="label" color="inherit">
                {option.label}
              </NexText>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
