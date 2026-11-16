import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, useLocation, useNavigate, useSearchParams } from "react-router-dom";

import { useAuth } from "@/auth/AuthProvider";
import { useBranding } from "@/branding/BrandingProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { changeLanguage } from "@/locales/i18n";
import { ApiError } from "@/requests/core/client";
import { listSSOConnections, startSSOLogin, type SSOConnection } from "@/requests/auth";
import {
  defaultLanguage,
  isAppLanguage,
  languageOptions,
} from "@/locales/languages";

import "./style.css";

// Maps the backend's SSO failure codes (server/system/sso.go) to the short,
// safe messages required by docs/System/SSO.md "Error Handling".
const ssoErrorKeys: Record<string, string> = {
  account_not_allowed: "login.k_Sso_Error_NotAllowed",
  account_pending: "login.k_Sso_Error_Pending",
  connection_disabled: "login.k_Sso_Error_Disabled",
  provider_unavailable: "login.k_Sso_Error_Unavailable",
  rate_limited: "login.k_Sso_Error_RateLimited",
};

function ssoErrorKey(code: string | null | undefined) {
  return (code && ssoErrorKeys[code]) || "login.k_Sso_Error_SignInFailed";
}

export default function Login() {
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  const { isAuthenticated, signIn } = useAuth();
  const branding = useBranding();
  const { i18n, t } = useTranslation("ui");
  useDocumentTitle(t("login.k_Main_PageTitle"));
  const currentLanguage = i18n.resolvedLanguage ?? null;
  const selectedLanguage = isAppLanguage(currentLanguage) ? currentLanguage : defaultLanguage;
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [loginError, setLoginError] = useState("");
  const [ssoConnections, setSsoConnections] = useState<SSOConnection[]>([]);
  const [ssoRedirecting, setSsoRedirecting] = useState<SSOConnection | null>(null);
  const [ssoErrorCode, setSsoErrorCode] = useState<string | null>(() => searchParams.get("sso_error"));
  const fromState = (location.state as { from?: { pathname?: string; search?: string } } | null)?.from;
  const returnTo = fromState?.pathname ? `${fromState.pathname}${fromState.search ?? ""}` : undefined;

  useEffect(() => {
    let isMounted = true;
    listSSOConnections()
      .then((items) => {
        if (isMounted) setSsoConnections(items);
      })
      .catch(() => {
        // No SSO (or no backend in dev): password login stays available.
        if (isMounted) setSsoConnections([]);
      });
    return () => {
      isMounted = false;
    };
  }, []);

  useEffect(() => {
    // Drop the failure code from the address bar so a reload doesn't show it again.
    if (searchParams.has("sso_error")) {
      const next = new URLSearchParams(searchParams);
      next.delete("sso_error");
      setSearchParams(next, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  async function handleSsoLogin(connection: SSOConnection) {
    setSsoErrorCode(null);
    setLoginError("");
    setSsoRedirecting(connection);
    try {
      window.location.assign(await startSSOLogin(connection.id, returnTo));
    } catch (error) {
      const code = error instanceof ApiError && typeof error.body?.code === "string" ? error.body.code : "provider_unavailable";
      setSsoErrorCode(code);
      setSsoRedirecting(null);
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setIsSubmitting(true);
    setLoginError("");
    const form = new FormData(event.currentTarget);
    try {
      await signIn({
        email: String(form.get("email") ?? ""),
        password: String(form.get("password") ?? ""),
      });
      navigate(returnTo ?? "/dashboard");
    } catch (error) {
      setLoginError(error instanceof Error ? error.message : "Login failed");
    } finally {
      setIsSubmitting(false);
    }
  }

  if (isAuthenticated) {
    return <Navigate to="/dashboard" replace />;
  }

  return (
    <main className="login-page">
      <div className="login-page__tools">
        <div className="login-page__language">
          <img
            className="login-page__language-icon"
            src={branding.image("language-icon.svg")}
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
          src={branding.image("login-logo.svg")}
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
            id="email"
            name="email"
            type="email"
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
          {loginError ? (
            <NexText className="login-form__error" variant="label" role="alert">
              {loginError}
            </NexText>
          ) : null}
          <NexButton type="submit" fullWidth disabled={isSubmitting || ssoRedirecting !== null}>
            {t("login.k_Main_Button_Login")}
          </NexButton>
        </form>

        {ssoConnections.length > 0 || ssoErrorCode ? (
          <div className="login-sso">
            {ssoConnections.length > 0 ? (
              <div className="login-sso__divider" aria-hidden="true">
                <NexText as="span" variant="label" color="muted">
                  {t("login.k_Sso_Divider_Or")}
                </NexText>
              </div>
            ) : null}
            {ssoErrorCode ? (
              <NexText className="login-form__error" variant="label" role="alert">
                {t(ssoErrorKey(ssoErrorCode))}
              </NexText>
            ) : null}
            {ssoConnections.map((connection) => (
              <NexButton
                key={connection.id}
                type="button"
                variant="secondary"
                fullWidth
                disabled={ssoRedirecting !== null || isSubmitting}
                onClick={() => void handleSsoLogin(connection)}
              >
                {ssoRedirecting?.id === connection.id
                  ? t("login.k_Sso_Status_Redirecting", { name: connection.name })
                  : t("login.k_Sso_Button_SignInWith", { name: connection.name })}
              </NexButton>
            ))}
          </div>
        ) : null}
      </section>
    </main>
  );
}
