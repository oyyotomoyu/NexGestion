import type { ButtonHTMLAttributes, ReactNode } from "react";

import { NexText } from "@/components/NexText";

import "./style.css";

interface NexButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  children: ReactNode;
  variant?: "primary" | "secondary" | "danger";
  size?: "default" | "compact";
  fullWidth?: boolean;
}

export function NexButton({
  children,
  className = "",
  variant = "primary",
  size = "default",
  fullWidth = false,
  ...props
}: NexButtonProps) {
  const classes = [
    "nex-button",
    `nex-button--${variant}`,
    `nex-button--${size}`,
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
