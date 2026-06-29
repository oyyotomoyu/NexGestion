import type { FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";

import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { changeLanguage } from "@/locales/i18n";
import {
  defaultLanguage,
  isAppLanguage,
  languageOptions,
} from "@/locales/languages";
import languageIcon from "@odm/img/language-icon.svg";
import loginLogo from "@odm/img/login-logo.svg";

import "./style.css";

export default function Login() {
  const navigate = useNavigate();
  const { i18n, t } = useTranslation("ui");
  useDocumentTitle(t("login.k_Main_PageTitle"));
  const currentLanguage = i18n.resolvedLanguage ?? null;
  const selectedLanguage = isAppLanguage(currentLanguage) ? currentLanguage : defaultLanguage;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    navigate("/dashboard");
  }

  return (
    <main className="login-page">
      <div className="login-page__tools">
        <div className="login-page__language">
          <img
            className="login-page__language-icon"
            src={languageIcon}
            alt=""
            aria-hidden="true"
          />
          <NexSelect
            ariaLabel={t("global.k_Language_Select")}
            value={selectedLanguage}
            options={languageOptions.map((language) => ({
              value: language.code,
              label: language.nativeName,
            }))}
            onChange={(language) => void changeLanguage(language)}
          />
        </div>
      </div>

      <section className="login-card" aria-labelledby="login-title">
        <img
          className="login-card__logo"
          src={loginLogo}
          alt={t("login.k_Main_ImageAlt_LoginLogo")}
        />
        <div className="login-card__heading">
          <NexText className="login-card__title" id="login-title" variant="heading">
            {t("login.k_Main_Title_WelcomeBack")}
          </NexText>
          <NexText variant="label" color="muted">
            {t("login.k_Main_Content_Instruction")}
          </NexText>
        </div>

        <form className="login-form" onSubmit={handleSubmit}>
          <NexInput
            id="username"
            name="username"
            label={t("login.k_Main_Label_Username")}
            placeholder={t("login.k_Main_Placeholder_Username")}
            autoComplete="username"
            autoFocus
            required
          />
          <NexInput
            id="password"
            name="password"
            type="password"
            label={t("login.k_Main_Label_Password")}
            placeholder={t("login.k_Main_Placeholder_Password")}
            autoComplete="current-password"
            required
          />
          <NexButton type="submit" fullWidth>
            {t("login.k_Main_Button_Login")}
          </NexButton>
        </form>
      </section>
    </main>
  );
}
