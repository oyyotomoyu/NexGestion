# NexGestion UI Architecture

## 1. Purpose

This document defines the planned frontend architecture for NexGestion.

The frontend will be installed and developed inside `client/` as a React application. The structure should reference the existing `xinsight/src/dashboard` project so the code organization feels familiar and maintainable.

## 2. Reference Project

Reference source:

```txt
/Users/yu/Documents/code/xinsight/src/dashboard
```

Observed patterns from xinsight:

- React 18 with TypeScript
- Vite as the frontend build tool
- `src/views` for route-level pages
- `src/layouts` for shared page shells and navigation layouts
- `src/components` for reusable UI components
- `src/requests` for API modules
- `src/locales` for i18n setup and translation keys
- `src/store` for Redux Toolkit state management
- `src/theme` for global styles and theme tokens
- `src/hooks` for reusable React hooks
- `src/utils` for non-UI utility functions
- `src/assets` for images, icons, and static UI assets

NexGestion should follow the same general separation, but simplify it for the first version.

## 3. Recommended Client Structure

```txt
client/
├── env/                     # Environment variables for dev/build modes
├── public/                  # Public static files
├── src/
│   ├── assets/              # Images, icons, fonts, and static frontend assets
│   ├── components/          # Shared reusable components
│   │   ├── base/            # Basic UI wrappers, buttons, inputs, tables
│   │   ├── feedback/        # Empty states, loading states, messages, errors
│   │   └── business/        # Reusable business-domain components
│   ├── hooks/               # Shared React hooks
│   ├── layouts/             # App shell, sidebar, header, auth layout
│   ├── locales/             # i18n setup and translation keys
│   │   └── key/             # Translation key definitions by domain
│   ├── requests/            # API request modules
│   │   ├── core/            # Request client, handlers, error normalization
│   │   └── modules/         # Business API modules
│   ├── store/               # Redux Toolkit store, slices, typed hooks
│   │   ├── slices/          # Feature state slices
│   │   └── interface/       # Shared store types
│   ├── theme/               # Global style, variables, design tokens
│   ├── utils/               # Pure utility functions
│   ├── views/               # Route-level pages
│   ├── main.tsx             # React entry point
│   └── App.tsx              # App providers and route mounting
├── index.html
├── package.json
├── tsconfig.json
└── vite.config.ts
```

Note: The current NexGestion folder has `client/src/request`. To stay closer to xinsight, the recommended final name is `client/src/requests`.

## 4. Application Entry

NexGestion should follow the same provider layering idea as xinsight:

```txt
main.tsx
└── React.StrictMode
    └── ThemeProvider
        └── Redux Provider
            └── Router
                └── App
```

Recommended providers:

- React `StrictMode`
- Theme provider
- Redux `Provider`
- React Router provider
- i18n initialization before rendering

For routing, xinsight uses `HashRouter`. NexGestion can also start with `HashRouter` because it is simple for local-first desktop or self-hosted deployment. This can be revisited if server-side routing becomes important.

## 5. Views

`src/views` should contain page-level features. A view owns the screen composition, route config, and page-specific components.

Recommended first structure:

```txt
src/views/
├── routes.tsx
├── index.tsx
├── Login/
│   ├── index.tsx
│   └── routes.tsx
├── Dashboard/
│   ├── index.tsx
│   ├── routes.tsx
│   └── components/
├── Customers/
│   ├── index.tsx
│   ├── routes.tsx
│   └── components/
├── Products/
│   ├── index.tsx
│   ├── routes.tsx
│   └── components/
├── Sales/
│   ├── index.tsx
│   ├── routes.tsx
│   └── components/
└── Settings/
    ├── index.tsx
    ├── routes.tsx
    └── components/
```

Guidelines:

- Keep route-level components in `views`.
- Keep page-only child components under that view's `components/`.
- Move reusable components to `src/components` only after they are shared by multiple views.
- Each business module should own its own route file.

## 6. Layouts

`src/layouts` should contain shared shells and navigation structure.

Recommended first structure:

```txt
src/layouts/
├── AppLayout/
│   ├── index.tsx
│   ├── Sidebar.tsx
│   ├── Header.tsx
│   └── style.scss
├── AuthLayout/
│   └── index.tsx
└── components/
```

Guidelines:

- `AppLayout` wraps authenticated pages.
- `AuthLayout` wraps login and account recovery pages.
- Sidebar and header state should be managed through store or layout-level state depending on scope.
- Layouts should not contain business-specific form or table logic.

## 7. Components

`src/components` should contain reusable UI building blocks.

Recommended categories:

```txt
src/components/
├── base/        # Button, Input, Select, Modal, Table wrappers
├── feedback/    # Loading, EmptyState, ErrorMessage, ConfirmDialog
├── business/    # CustomerPicker, MoneyText, StatusTag
└── templates/   # Reusable page or form templates
```

Guidelines:

- `base` components should stay domain-neutral.
- `business` components may know NexGestion business concepts.
- Avoid placing page-only components here too early.
- Component names should be clear and consistent, such as `CustomerPicker`, `EmptyState`, `PageHeader`.

## 8. Requests

`src/requests` should follow xinsight's dedicated API module approach.

Recommended structure:

```txt
src/requests/
├── core/
│   ├── client.ts        # Axios/fetch wrapper
│   ├── errors.ts        # Error normalization
│   └── handlers.ts      # Default response handlers
├── auth.ts
├── customers.ts
├── products.ts
├── sales.ts
├── invoices.ts
└── system.ts
```

Guidelines:

- Views should not call `fetch` or `axios` directly.
- Request modules should expose typed functions such as `listCustomers()` or `createCustomer()`.
- Request error shapes should be normalized in `requests/core`.
- Backend API paths should stay centralized in request modules.
- Auth token handling should be isolated in the request layer.

## 9. Locales

`src/locales` should follow xinsight's i18n direction with translation keys grouped by domain.

Recommended structure:

```txt
src/locales/
├── i18n.ts
└── key/
    ├── Global.ts
    ├── Component.ts
    ├── Auth.ts
    ├── Dashboard.ts
    ├── Customers.ts
    ├── Products.ts
    ├── Sales.ts
    ├── Invoices.ts
    └── Settings.ts
```

Initial language direction:

- Start with English keys and Traditional Chinese support if needed.
- Keep shared terms in `Global` or `Component`.
- Keep business module copy in module-specific files.
- Avoid hard-coded UI strings in views and components once i18n is introduced.

## 10. Store

`src/store` should use Redux Toolkit if NexGestion follows xinsight closely.

Recommended structure:

```txt
src/store/
├── index.ts
├── interface/
└── slices/
    ├── auth.ts
    ├── app.ts
    ├── ui.ts
    ├── customers.ts
    └── settings.ts
```

Guidelines:

- Store global state only when it is shared across multiple screens.
- Keep temporary form state inside components or form libraries.
- Use typed hooks such as `useAppDispatch` and `useAppSelector`.
- Keep API response caching strategy separate from UI-only state decisions.

## 11. Hooks

`src/hooks` should contain reusable React hooks.

Possible first hooks:

- `useAbortableRequest`
- `useDebounce`
- `usePagination`
- `useSearchParams`
- `useLocalPreference`

Guidelines:

- Hooks should be reusable and not tied to a single view unless placed inside that view.
- Request cancellation and polling should be centralized here when needed.

## 12. Theme

`src/theme` should contain global styles and design tokens.

Recommended structure:

```txt
src/theme/
├── global.scss
├── variable.scss
└── tokens.ts
```

Guidelines:

- Keep colors, spacing, typography, and shared layout values centralized.
- Avoid scattering hard-coded colors across views.
- Keep theme naming business-neutral.

## 13. Assets

`src/assets` should contain static files used by the React app.

Recommended structure:

```txt
src/assets/
├── icons/
├── images/
└── fonts/
```

Guidelines:

- Use SVG for icons when possible.
- Keep product or brand assets separated from generic UI assets.
- Avoid storing generated or temporary files here.

## 14. First Implementation Plan

1. Initialize React with Vite and TypeScript inside `client`.
2. Add the base folder structure from this document.
3. Add `main.tsx`, app routing, theme, and i18n bootstrap.
4. Add `layouts/AppLayout` and `layouts/AuthLayout`.
5. Add initial views: `Login`, `Dashboard`, and `Settings`.
6. Add `requests/core` and a `system.ts` API module for `/api/health`.
7. Add Redux Toolkit store with `auth`, `app`, and `ui` slices.

## 15. Open Decisions

- Should NexGestion use Redux Toolkit from the start, or start with React state and add Redux later?
- Should the first UI use Ant Design, another component library, or custom components?
- Should routing use `HashRouter` like xinsight or `BrowserRouter`?
- Should API calls use Axios like xinsight or native `fetch`?
- Which languages should be supported in the first i18n setup?
