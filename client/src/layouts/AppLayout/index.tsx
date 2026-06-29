import { NavLink, Outlet } from "react-router-dom";

import { NexText } from "@/components/NexText";

import "./style.css";

export default function AppLayout() {
  return (
    <div className="app-layout">
      <aside className="app-sidebar">
        <NexText className="app-brand" variant="brand">
          NexGestion
        </NexText>
        <nav className="app-nav">
          <NavLink to="/dashboard">
            <NexText as="span" variant="label" color="inherit">
              Dashboard
            </NexText>
          </NavLink>
          <NavLink to="/settings">
            <NexText as="span" variant="label" color="inherit">
              Settings
            </NexText>
          </NavLink>
        </nav>
      </aside>
      <main className="app-main">
        <Outlet />
      </main>
    </div>
  );
}
