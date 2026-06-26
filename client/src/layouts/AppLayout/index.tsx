import { NavLink, Outlet } from "react-router-dom";

import "./style.css";

export default function AppLayout() {
  return (
    <div className="app-layout">
      <aside className="app-sidebar">
        <div className="app-brand">NexGestion</div>
        <nav className="app-nav">
          <NavLink to="/dashboard">Dashboard</NavLink>
          <NavLink to="/settings">Settings</NavLink>
        </nav>
      </aside>
      <main className="app-main">
        <Outlet />
      </main>
    </div>
  );
}
