import React from "react";
import ReactDOM from "react-dom/client";
import { HashRouter } from "react-router-dom";

import App from "@/views";
import { NexColor } from "@/components/NexColor";
import "@/theme/global.css";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <NexColor>
      <HashRouter>
        <App />
      </HashRouter>
    </NexColor>
  </React.StrictMode>
);
