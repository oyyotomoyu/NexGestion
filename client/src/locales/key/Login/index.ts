import CHT from "./CHT.json";
import ENG from "./ENG.json";
import JPN from "./JPN.json";

import type { AppLanguage } from "@/locales/languages";

const loginResources = {
  CHT,
  ENG,
  JPN,
};

export default function getLoginResources(language: AppLanguage) {
  return loginResources[language] ?? ENG;
}
