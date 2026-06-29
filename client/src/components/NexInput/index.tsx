import { forwardRef, type InputHTMLAttributes } from "react";

import { NexText } from "@/components/NexText";

import "./style.css";

interface NexInputProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
}

export const NexInput = forwardRef<HTMLInputElement, NexInputProps>(function NexInput(
  { className = "", id, label, ...props },
  ref,
) {
  return (
    <label className={`nex-input ${className}`} htmlFor={id}>
      <NexText className="nex-input__label" as="span" variant="label">
        {label}
      </NexText>
      <input ref={ref} className="nex-input__control" id={id} {...props} />
    </label>
  );
});
