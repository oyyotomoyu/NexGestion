import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";

import "./style.css";

export default function Settings() {
  const { t } = useTranslation("ui");
  useDocumentTitle(t("global.k_Settings_PageTitle"));

  return (
    <section className="settings">
      <header className="settings__header">
        <NexText variant="title">{t("global.k_Settings_Title")}</NexText>
        <NexText color="muted">{t("global.k_Settings_AccessControl_Description")}</NexText>
      </header>
      <div className="settings__layout">
        <nav className="settings__nav" aria-label={t("global.k_Settings_AccessControl_Title")}>
          <NexText variant="label">{t("global.k_Settings_AccessControl_Title")}</NexText>
          <NavLink to="access-control/roles">
            <NexText as="span" color="inherit">{t("global.k_Settings_Roles_Title")}</NexText>
          </NavLink>
          <NavLink to="access-control/groups">
            <NexText as="span" color="inherit">{t("global.k_Settings_Groups_Title")}</NexText>
          </NavLink>
        </nav>
        <div className="settings__content">
          <Outlet />
        </div>
      </div>
    </section>
  );
}
