import React from "react";
import ReactDOM from "react-dom/client";
import { HashRouter } from "react-router-dom";

import App from "@/views";
import { NexColor } from "@/components/NexColor";
import faviconUrl from "@odm/img/favicon.ico?url";
import "@/locales/i18n";
import "@/theme/global.css";

const favicon = document.createElement("link");
favicon.rel = "icon";
favicon.type = "image/x-icon";
favicon.sizes = "32x32";
favicon.href = faviconUrl;
document.head.append(favicon);

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <NexColor>
      <HashRouter>
        <App />
      </HashRouter>
    </NexColor>
  </React.StrictMode>
);
