import CHT from "./CHT.json";
import ENG from "./ENG.json";
import JPN from "./JPN.json";

import type { AppLanguage } from "@/locales/languages";

const globalResources = {
  CHT,
  ENG,
  JPN,
};

export default function getGlobalResources(language: AppLanguage) {
  return globalResources[language] ?? ENG;
}
