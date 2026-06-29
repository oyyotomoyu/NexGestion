import { noI18n } from "@/locales/noI18n";

export const supportedLanguages = ["ENG", "CHT", "JPN"] as const;

export type AppLanguage = (typeof supportedLanguages)[number];

export const defaultLanguage: AppLanguage = "ENG";

export const htmlLanguages: Record<AppLanguage, string> = {
  CHT: "zh-Hant",
  ENG: "en",
  JPN: "ja",
};

export const languageOptions: Array<{
  code: AppLanguage;
  nativeName: string;
}> = [
  { code: "ENG", nativeName: noI18n("English") },
  { code: "CHT", nativeName: noI18n("正體中文") },
  { code: "JPN", nativeName: noI18n("日本語") },
];

export function isAppLanguage(language: string | null): language is AppLanguage {
  return supportedLanguages.some((supportedLanguage) => supportedLanguage === language);
}
