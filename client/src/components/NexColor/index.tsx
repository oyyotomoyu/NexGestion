import { createContext, useContext, type CSSProperties, type ReactNode } from "react";

import defaultThemeJson from "@odm/theme.json";

export interface NexColors {
  primary: string;
  primaryHover: string;
  primarySoft: string;
  secondary: string;
  secondaryHover: string;
  secondarySoft: string;
  text: string;
  heading: string;
  content: string;
  muted: string;
  onPrimary: string;
  background: string;
  surface: string;
  surfaceMuted: string;
  border: string;
  focusRing: string;
  shadow: string;
  success: string;
  warning: string;
  danger: string;
  info: string;
}

export interface NexColorTheme {
  name: string;
  colors: NexColors;
}

interface NexColorProps {
  children: ReactNode;
  theme?: NexColorTheme;
}

type NexColorVariables = CSSProperties & Record<`--color-${string}`, string>;

const defaultTheme = defaultThemeJson satisfies NexColorTheme;
const NexColorContext = createContext<NexColorTheme>(defaultTheme);

function toCssVariableName(token: string) {
  return token.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
}

export function getNexColorVariable(token: keyof NexColors) {
  return `var(--color-${toCssVariableName(token)})`;
}

function createColorVariables(colors: NexColors): NexColorVariables {
  return Object.fromEntries(
    Object.entries(colors).map(([token, value]) => [
      `--color-${toCssVariableName(token)}`,
      value,
    ]),
  ) as NexColorVariables;
}

export function NexColor({ children, theme = defaultTheme }: NexColorProps) {
  return (
    <NexColorContext.Provider value={theme}>
      <div
        className="theme-root"
        data-theme={theme.name}
        style={createColorVariables(theme.colors)}
      >
        {children}
      </div>
    </NexColorContext.Provider>
  );
}

export function useNexColor() {
  return useContext(NexColorContext);
}
