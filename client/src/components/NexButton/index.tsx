import type { ButtonHTMLAttributes, ReactNode } from "react";

import { NexText } from "@/components/NexText";

import "./style.css";

interface NexButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  children: ReactNode;
  variant?: "primary" | "secondary";
  fullWidth?: boolean;
}

export function NexButton({
  children,
  className = "",
  variant = "primary",
  fullWidth = false,
  ...props
}: NexButtonProps) {
  const classes = [
    "nex-button",
    `nex-button--${variant}`,
    fullWidth ? "nex-button--full-width" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <button className={classes} {...props}>
      <NexText as="span" variant="button" color="inherit">
        {children}
      </NexText>
    </button>
  );
}
