# NexGestion
An Open-source local-first business management platform for SMEs.

# NexGestion

**NexGestion** is an open-source, local-first business management platform designed for small and medium-sized businesses (SMEs).

The goal of NexGestion is to provide a simple, self-hosted, and modular alternative to traditional cloud-based enterprise software. Organizations can deploy the platform on a laptop, mini PC, or local server, allowing employees to access business services through their mobile devices or web browsers within the local network.

## Vision

Digital transformation should not require expensive subscriptions, complex infrastructure, or dependence on cloud providers.

NexGestion aims to help small businesses, restaurants, workshops, retail stores, schools, clubs, and local organizations manage their daily operations with a lightweight and flexible platform that they fully control.

## Core Principles

### Local-First

Run business systems on your own devices and local network.

### Open Source

Transparent, customizable, and community-driven.

### Modular Architecture

Install only the modules your organization needs.

### Self-Hosted

Own your data and infrastructure.

## Roadmap

NexGestion is designed as a platform consisting of multiple business modules.

### Foundation Modules

- User Management
- Authentication
- Role Management
- Permission Management
- Organization Management
- Team Management
- Member Management

### Human Resources (HR)

- Attendance Tracking
- Leave Management
- Employee Directory
- Shift Scheduling
- Employee Profiles

### Financial Management

- Expense Tracking
- Budget Management
- Invoice Management
- Financial Reports

### Warehouse & Inventory

- Inventory Tracking
- Stock Management
- Purchase Records
- Supplier Management

### Order Management

- Order Tracking
- Customer Management
- Product Catalog
- Sales Reports

### Workspace Management

- Meeting Room Reservation
- Equipment Reservation
- Shared Resource Scheduling
- Office Space Booking

### Additional Modules

- Internal Announcement System
- Task Management
- Approval Workflow
- Asset Management
- Restaurant Ordering System
- Visitor Registration
- Custom Extensions

## Planned Architecture

```text
NexGestion
├── Core
│   ├── Authentication
│   ├── User Management
│   ├── Role Management
│   ├── Permission Management
│   └── Organization Management
│
├── HR Module
├── Finance Module
├── Inventory Module
├── Workspace Module
├── Order Module
└── Plugin System



## Technology Stack

NexGestion is built with modern, cross-platform technologies to ensure simple deployment and long-term maintainability.

### Frontend

- React
- TypeScript
- Vite
- Ant Design (planned)

### Backend

- Go (Golang)
- RESTful API
- JWT Authentication (planned)

### Database

- SQLite (default)

### Deployment

- Windows
- macOS
- Linux

## Architecture

```text
┌─────────────────┐
│   Mobile/Web    │
│     Browser     │
└────────┬────────┘
         │ HTTP
         ▼
┌─────────────────┐
│ NexGestion API  │
│    Go Server    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│     SQLite      │
│    Database     │
└─────────────────┘
```

## Development Requirements

### Frontend

- Node.js 22+
- npm or pnpm

### Backend

- Go 1.24+

### Database

- SQLite 3+

## Getting Started

### Clone Repository

```bash
git clone https://github.com/<your-account>/NexGestion.git
cd NexGestion
```

### Frontend Setup

```bash
cd frontend
npm install
npm run dev
```

or

```bash
pnpm install
pnpm dev
```

Frontend will start at:

```text
http://localhost:5173
```

### Backend Setup

```bash
cd backend
go mod tidy
go run main.go
```

Backend API will start at:

```text
http://localhost:8080
```

### Production Build

#### Frontend

```bash
npm run build
```

#### Backend

```bash
go build -o nexgestion
```

## Future Installation Experience

The long-term goal of NexGestion is to provide a zero-configuration installation experience.

Users should be able to:

1. Download the installer.
2. Run the executable.
3. Automatically initialize the database.
4. Open the management portal.
5. Start using the system immediately.

No manual installation of:

- Go
- Node.js
- SQLite
- Apache
- Nginx
- Docker

should be required for end users.

## Future Roadmap

### Core Platform

- Authentication
- User Management
- Role Management
- Permission Management
- Organization Management
- Team Management

### HR System

- Attendance Tracking
- Leave Requests
- Employee Directory
- Shift Scheduling

### Finance System

- Expense Tracking
- Budget Management
- Invoice Management

### Inventory System

- Product Management
- Stock Tracking
- Warehouse Management

### Workspace System

- Meeting Room Reservation
- Equipment Reservation
- Resource Scheduling

### Order System

- Customer Management
- Order Tracking
- Product Catalog

### Plugin System

- Third-party extensions
- Custom modules
- Community contributions