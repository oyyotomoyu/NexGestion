import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import getGlobalResources from "@/locales/key/Global";
import getLoginResources from "@/locales/key/Login";
import {
  defaultLanguage,
  htmlLanguages,
  isAppLanguage,
  supportedLanguages,
  type AppLanguage,
} from "@/locales/languages";

export { noI18n } from "@/locales/noI18n";

const storedLanguage = window.localStorage.getItem("language");
const initialLanguage = isAppLanguage(storedLanguage) ? storedLanguage : defaultLanguage;

const resources = Object.fromEntries(
  supportedLanguages.map((language) => [
    language,
    {
      ui: {
        global: getGlobalResources(language),
        login: getLoginResources(language),
      },
    },
  ]),
);

void i18n.use(initReactI18next).init({
  resources,
  lng: initialLanguage,
  fallbackLng: "ENG",
  supportedLngs: supportedLanguages,
  load: "currentOnly",
  ns: ["ui"],
  defaultNS: "ui",
  interpolation: {
    escapeValue: false,
  },
});

document.documentElement.lang = htmlLanguages[initialLanguage];

export async function changeLanguage(language: AppLanguage) {
  window.localStorage.setItem("language", language);
  document.documentElement.lang = htmlLanguages[language];
  await i18n.changeLanguage(language);
}

export default i18n;
