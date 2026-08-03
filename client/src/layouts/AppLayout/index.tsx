import { useEffect, useRef, useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
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
  const location = useLocation();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const menuWasOpen = useRef(false);
  const currentLanguage = i18n.resolvedLanguage ?? null;
  const selectedLanguage = isAppLanguage(currentLanguage) ? currentLanguage : defaultLanguage;

  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    if (!menuOpen) {
      if (menuWasOpen.current) {
        menuButtonRef.current?.focus();
        menuWasOpen.current = false;
      }
      return;
    }
    menuWasOpen.current = true;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeButtonRef.current?.focus();
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") setMenuOpen(false);
    }
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [menuOpen]);

  return (
    <div className="app-layout">
      <header className="app-mobile-header">
        <NexText className="app-mobile-header__brand" variant="brand">
          {t("global.k_App_Brand")}
        </NexText>
        <button
          ref={menuButtonRef}
          className="app-menu-button"
          type="button"
          aria-expanded={menuOpen}
          aria-controls="app-sidebar"
          aria-label={t("global.k_Nav_OpenMenu")}
          onClick={() => setMenuOpen(true)}
        >
          <span />
          <span />
          <span />
        </button>
      </header>
      <button
        className={`app-sidebar-backdrop${menuOpen ? " is-open" : ""}`}
        type="button"
        aria-label={t("global.k_Nav_CloseMenu")}
        tabIndex={menuOpen ? 0 : -1}
        onClick={() => setMenuOpen(false)}
      />
      <aside id="app-sidebar" className={`app-sidebar${menuOpen ? " is-open" : ""}`}>
        <div>
          <div className="app-sidebar__heading">
            <NexText className="app-brand" variant="brand">
              {t("global.k_App_Brand")}
            </NexText>
            <button
              ref={closeButtonRef}
              className="app-sidebar__close"
              type="button"
              aria-label={t("global.k_Nav_CloseMenu")}
              onClick={() => setMenuOpen(false)}
            >
              <span />
              <span />
            </button>
          </div>
          <nav className="app-nav">
            <NavLink to="/dashboard" onClick={() => setMenuOpen(false)}>
              <NexText as="span" variant="label" color="inherit">
                {t("global.k_Nav_Dashboard")}
              </NexText>
            </NavLink>
            <NavLink to="/notifications" onClick={() => setMenuOpen(false)}>
              <NexText as="span" variant="label" color="inherit">
                {t("global.k_Nav_Notifications")}
              </NexText>
            </NavLink>
            <NavLink end to="/attendance" onClick={() => setMenuOpen(false)}>
              <NexText as="span" variant="label" color="inherit">
                {t("global.k_Nav_Attendance")}
              </NexText>
            </NavLink>
            <NavLink to="/attendance/approvals" onClick={() => setMenuOpen(false)}>
              <NexText as="span" variant="label" color="inherit">
                {t("global.k_Nav_LeaveApprovals")}
              </NexText>
            </NavLink>
            <NavLink to="/settings" onClick={() => setMenuOpen(false)}>
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
