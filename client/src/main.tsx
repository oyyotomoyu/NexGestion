import React from "react";
import ReactDOM from "react-dom/client";
import { HashRouter } from "react-router-dom";

import { AuthProvider } from "@/auth/AuthProvider";
import App from "@/views";
import { BrandingProvider } from "@/branding/BrandingProvider";
import "@/locales/i18n";
import "@/theme/global.css";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <BrandingProvider>
      <HashRouter>
        <AuthProvider>
          <App />
        </AuthProvider>
      </HashRouter>
    </BrandingProvider>
  </React.StrictMode>
);
