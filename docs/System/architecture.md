# NexGestion System Architecture

## 1. Overview

NexGestion is planned as a local-first business management platform for small and medium-sized businesses.

The system will be separated into two main applications:

- `client`: Frontend application built with React
- `server`: Backend application built with Go

The first architecture goal is to keep the system simple, modular, and easy to evolve while the product scope is still being defined.

## 2. Architecture Goals

- Keep business data available in local or self-hosted environments
- Separate frontend UI concerns from backend business logic
- Define clear API boundaries between `client` and `server`
- Support future modules such as customers, inventory, sales, invoicing, and reporting
- Keep the early codebase easy to understand for future contributors

## 3. Repository Structure

```txt
NexGestion/
├── client/              # React frontend application
│   └── src/
│       ├── assets/      # Static frontend assets
│       ├── components/  # Reusable UI components
│       ├── hooks/       # Shared React hooks
│       ├── layouts/     # Shared app layouts and navigation shells
│       ├── locales/     # Translation and localization files
│       ├── requests/    # API request clients
│       ├── store/       # Frontend state management
│       ├── theme/       # UI theme tokens and styles
│       ├── utils/       # Shared frontend utilities
│       └── views/       # Page-level views
├── server/              # Go backend application
│   ├── apis/            # API handlers and route definitions
│   ├── system/          # Core system services and domain logic
│   └── tools/           # Backend tools, scripts, and helpers
├── database/            # Database schema, migrations, or local data setup
├── config/              # Shared configuration examples
├── docs/                # Product and technical documentation
│   └── System/          # System architecture and technical decisions
├── installer/           # Platform-specific installer planning
├── odm/                 # Object/document mapping or data model planning
└── script/              # Project-level scripts
```

## 4. Frontend: `client`

The frontend will be implemented with React.

Primary responsibilities:

- Render the business management UI
- Manage page-level and component-level state
- Call backend APIs through a dedicated request layer
- Handle localization and user-facing formatting
- Provide workflows for planned business modules

Planned frontend structure:

- `src/views`: Route-level pages such as dashboard, customers, inventory, sales, and settings
- `src/components`: Reusable UI components shared across views
- `src/requests`: API client functions for communicating with the Go backend
- `src/store`: Client-side state management
- `src/theme`: Design tokens, theme settings, and shared styles
- `src/locales`: Translation files for future multilingual support
- `src/layouts`: Shared application shells, sidebar, header, and auth layouts
- `src/hooks`: Shared React hooks
- `src/utils`: Small frontend-only utility functions

Initial frontend decisions still pending:

- React framework or bundler: Vite, Next.js, or another option
- Routing library
- UI component strategy
- State management approach
- Form and validation libraries

## 5. Backend: `server`

The backend will be implemented with Go.

Primary responsibilities:

- Expose APIs for the React frontend
- Own business rules and validation
- Manage data persistence
- Handle authentication and authorization
- Provide import, export, and backup-related services in the future

Planned backend structure:

- `server/apis`: HTTP handlers, route registration, request parsing, and response formatting
- `server/system`: Core services, domain logic, permission checks, and business workflows
- `server/tools`: Internal backend utilities, generators, maintenance helpers, or development tools

Initial backend decisions still pending:

- HTTP framework or router
- Database engine
- Migration strategy
- Authentication strategy
- Logging and error handling conventions
- Configuration loading approach

## 6. API Boundary

The frontend should communicate with the backend only through documented APIs.

Planned API style:

- REST-style HTTP APIs for the first version
- JSON request and response payloads
- Versioned routes if public API stability becomes important
- Consistent error response format

Example route direction:

```txt
GET    /api/health
GET    /api/customers
POST   /api/customers
GET    /api/products
POST   /api/products
GET    /api/orders
POST   /api/orders
```

The exact API contract should be documented before implementation starts.

## 7. Data Layer

NexGestion is local-first, so the data layer should be designed with offline availability and easy backup in mind.

Possible directions:

- Local database for single-machine or self-hosted usage
- Clear migration files under `database/`
- Export and backup support as a first-class design concern
- Future synchronization support if multi-device workflows become part of the scope

Pending decisions:

- Database choice
- Whether the backend owns all persistence
- Whether the frontend needs any offline cache beyond normal UI state
- Backup format and restore workflow

## 8. Planned Business Modules

The following modules are candidates for future implementation:

- Customers and contacts
- Products and inventory
- Sales and orders
- Invoices and payments
- Reports and business insights
- Users, roles, and permissions
- Settings and organization profile

Each module should be planned with:

- Domain model
- API contract
- UI workflows
- Permission requirements
- Import and export needs

## 9. Local-First Considerations

Local-first behavior should be considered from the beginning, even if the first implementation is simple.

Important questions:

- Can the system run without external cloud services?
- Where is the primary data stored?
- How are backups created and restored?
- How does the system behave without internet access?
- Will future sync be required between multiple devices or users?

## 10. Development Phases

### Phase 1: Planning

- Define product scope
- Choose frontend and backend technical stack
- Define repository conventions
- Draft core module requirements
- Draft initial API contracts

### Phase 2: Foundation

- Initialize React frontend in `client`
- Initialize Go backend in `server`
- Add development scripts
- Add health check API
- Add basic frontend routing
- Add initial configuration conventions

### Phase 3: First Business Module

- Select the first module to implement
- Define its data model
- Implement backend APIs
- Implement frontend views
- Add basic tests
- Document usage and development workflow

### Phase 4: Packaging And Local Deployment

- Define installer direction for macOS, Linux, and Windows
- Add local database setup
- Add backup and restore workflow
- Prepare first usable local release

## 11. Open Decisions

- Which React setup should be used?
- Which Go HTTP framework or router should be used?
- Which database should power the local-first backend?
- Should the first release target single-user usage or multi-user usage?
- What is the first business module to build?
- What license should the project use?

## 12. Next Documents To Create

- `docs/System/tech-stack.md`
- `docs/System/api-guidelines.md`
- `docs/System/data-model.md`
- `docs/System/auth-and-permissions.md`
- `docs/System/development-workflow.md`
