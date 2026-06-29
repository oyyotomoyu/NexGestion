import {
  createElement,
  type CSSProperties,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import {
  getNexColorVariable,
  useNexColor,
  type NexColors,
} from "@/components/NexColor";

import "./style.css";

export type NexTextVariant =
  | "display"
  | "title"
  | "heading"
  | "subheading"
  | "body"
  | "label"
  | "caption"
  | "button"
  | "brand";

type NexTextElement =
  | "span"
  | "p"
  | "div"
  | "label"
  | "h1"
  | "h2"
  | "h3"
  | "h4"
  | "h5"
  | "h6";

export interface NexTextProps extends Omit<HTMLAttributes<HTMLElement>, "color"> {
  children: ReactNode;
  as?: NexTextElement;
  variant?: NexTextVariant;
  color?: keyof NexColors | string;
  size?: CSSProperties["fontSize"];
  weight?: CSSProperties["fontWeight"];
  fontFamily?: CSSProperties["fontFamily"];
  lineHeight?: CSSProperties["lineHeight"];
  align?: CSSProperties["textAlign"];
  truncate?: boolean;
}

const defaultElements: Record<NexTextVariant, NexTextElement> = {
  display: "h1",
  title: "h1",
  heading: "h2",
  subheading: "h3",
  body: "p",
  label: "span",
  caption: "span",
  button: "span",
  brand: "span",
};

export function NexText({
  align,
  as,
  children,
  className = "",
  color,
  fontFamily,
  lineHeight,
  size,
  style,
  truncate = false,
  variant = "body",
  weight,
  ...props
}: NexTextProps) {
  const theme = useNexColor();
  const isThemeColor = color && Object.prototype.hasOwnProperty.call(theme.colors, color);
  const resolvedColor = isThemeColor
    ? getNexColorVariable(color as keyof NexColors)
    : color;
  const classes = [
    "nex-text",
    `nex-text--${variant}`,
    truncate ? "nex-text--truncate" : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  return createElement(
    as ?? defaultElements[variant],
    {
      ...props,
      className: classes,
      style: {
        ...style,
        color: resolvedColor ?? style?.color,
        fontFamily: fontFamily ?? style?.fontFamily,
        fontSize: size ?? style?.fontSize,
        fontWeight: weight ?? style?.fontWeight,
        lineHeight: lineHeight ?? style?.lineHeight,
        textAlign: align ?? style?.textAlign,
      },
    },
    children,
  );
}
