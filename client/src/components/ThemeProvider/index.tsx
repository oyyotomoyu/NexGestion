import { createContext, useContext, type CSSProperties, type ReactNode } from "react";

import defaultThemeJson from "@odm/theme.json";

export interface ThemeColors {
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
  success: string;
  warning: string;
  danger: string;
  info: string;
}

export interface ThemeConfig {
  name: string;
  colors: ThemeColors;
}

interface ThemeProviderProps {
  children: ReactNode;
  theme?: ThemeConfig;
}

type ThemeVariables = CSSProperties & Record<`--color-${string}`, string>;

const defaultTheme = defaultThemeJson satisfies ThemeConfig;
const ThemeContext = createContext<ThemeConfig>(defaultTheme);

function toCssVariableName(token: string) {
  return token.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
}

function createThemeVariables(colors: ThemeColors): ThemeVariables {
  return Object.fromEntries(
    Object.entries(colors).map(([token, value]) => [
      `--color-${toCssVariableName(token)}`,
      value,
    ]),
  ) as ThemeVariables;
}

export function ThemeProvider({ children, theme = defaultTheme }: ThemeProviderProps) {
  return (
    <ThemeContext.Provider value={theme}>
      <div
        className="theme-root"
        data-theme={theme.name}
        style={createThemeVariables(theme.colors)}
      >
        {children}
      </div>
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  return useContext(ThemeContext);
}
