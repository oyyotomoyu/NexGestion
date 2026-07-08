import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { changeLanguage } from "@/locales/i18n";
import {
  defaultLanguage,
  isAppLanguage,
  languageOptions,
} from "@/locales/languages";
import languageIcon from "@odm/img/language-icon.svg";

import "./style.css";

export default function AppLayout() {
  const { i18n, t } = useTranslation("ui");
  const currentLanguage = i18n.resolvedLanguage ?? null;
  const selectedLanguage = isAppLanguage(currentLanguage) ? currentLanguage : defaultLanguage;

  return (
    <div className="app-layout">
      <aside className="app-sidebar">
        <div>
          <NexText className="app-brand" variant="brand">
            {t("global.k_App_Brand")}
          </NexText>
          <nav className="app-nav">
            <NavLink to="/dashboard">
              <NexText as="span" variant="label" color="inherit">
                {t("global.k_Nav_Dashboard")}
              </NexText>
            </NavLink>
            <NavLink to="/settings">
              <NexText as="span" variant="label" color="inherit">
                {t("global.k_Nav_Settings")}
              </NexText>
            </NavLink>
          </nav>
        </div>
        <div className="app-sidebar__language">
          <img
            className="app-sidebar__language-icon"
            src={languageIcon}
            alt=""
            aria-hidden="true"
          />
          <NexSelect
            ariaLabel={t("global.k_Language_Select")}
            menuPlacement="top"
            value={selectedLanguage}
            options={languageOptions.map((language) => ({
              value: language.code,
              label: language.nativeName,
            }))}
            onChange={(language) => void changeLanguage(language)}
          />
        </div>
      </aside>
      <main className="app-main">
        <Outlet />
      </main>
    </div>
  );
}
