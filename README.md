# Openleaf Backend 🍃

[![Go Report Card](https://goreportcard.com/badge/github.com/saranshhalwai/openleaf-backend)](https://goreportcard.com/report/github.com/saranshhalwai/openleaf-backend)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/badge/release-v1.0.0-blue.svg)](https://github.com/saranshhalwai/openleaf-backend/releases)

The high-performance Go backend service for **Openleaf** — an open-source, modern collaborative LaTeX editor built with cloud-native scalability, real-time Yjs CRDT synchronization, and asynchronous sandboxed TeX Live compilation.

---

## ⚡ Features

- **RESTful API**: Fast and robust project, member, and file CRUD powered by [`go-chi`](https://github.com/go-chi/chi).
- **Real-Time Collaboration Hub**: In-memory WebSocket hub relaying binary Yjs CRDT protocol frames and user awareness states (max 6 concurrent editors per file room).
- **Role-Based Access Control**:
  - `owner`: Full administrative rights, collaborator management, project deletion.
  - `editor`: Read/write file access and collaborative sync.
  - `viewer`: Strictly enforced read-only access (rejected from WebSocket sync and mutations).
- **Asynchronous Compilation Pipeline**: Bundles project files into tarballs on Google Cloud Storage (GCS) and dispatches compile tasks to Cloud Tasks with authenticated callback webhooks.
- **Starter Templates**: Curated templates (Standard Article, IEEE Conference Paper with BibTeX, Resume/CV, Beamer Slides) pre-populated on project initialization.
- **Project Export**: Direct in-memory streaming ZIP archive generator (`/api/projects/{projectId}/export`).
- **Database Pooling**: High-throughput connection pooling with Neon Serverless PostgreSQL using [`pgx/v5`](https://github.com/jackc/pgx).

---

## 🏗️ Architecture

```
                       ┌────────────────────────────┐
                       │   Openleaf SvelteKit Web   │
                       └─────────────┬──────────────┘
                                     │
                 ┌───────────────────┴───────────────────┐
                 │                                       │
         [REST API Bearer]                      [WebSocket Binary]
                 │                                       │
                 ▼                                       ▼
        ┌──────────────────┐                   ┌──────────────────┐
        │  REST Handlers   │                   │  Collab Hub /    │
        │  (CRUD & Export) │                   │  Yjs Room Relay  │
        └────────┬─────────┘                   └────────┬─────────┘
                 │                                      │
                 ├──────────────────┬───────────────────┤
                 ▼                  ▼                   ▼
        ┌──────────────────┐ ┌──────────────┐ ┌───────────────────┐
        │ Neon PostgreSQL  │ │  GCS Bucket  │ │ Cloud Tasks Queue │
        │ (ap-southeast-1) │ │ (asia-se1)   │ │  (asia-se1)       │
        └──────────────────┘ └──────┬───────┘ └─────────┬─────────┘
                                    │                   │
                                    ▼                   ▼
                             ┌─────────────────────────────┐
                             │    Openleaf Compiler Run    │
                             │   (latexmk & TeX Live)      │
                             └─────────────────────────────┘
```

---

## 📋 API Endpoints

### Public & Authentication
- `GET /api/health` — Health check endpoint.
- `GET /api/me` — Retrieve current authenticated user profile.

### Projects & Templates
- `GET /api/templates` — List all available starter templates.
- `GET /api/projects` — List all projects accessible to the user.
- `POST /api/projects` — Create a new project (supports `template` parameter).
- `GET /api/projects/{projectId}` — Get project metadata and file tree.
- `DELETE /api/projects/{projectId}` — Delete project (owner only).
- `GET /api/projects/{projectId}/export` — Download complete project as `.zip`.

### Files
- `GET /api/projects/{projectId}/files` — List files in project.
- `POST /api/projects/{projectId}/files` — Create a new file.
- `GET /api/projects/{projectId}/files/{fileId}` — Read file content.
- `PUT /api/projects/{projectId}/files/{fileId}` — Update file content.
- `DELETE /api/projects/{projectId}/files/{fileId}` — Delete file.

### Collaboration & Real-Time Sync
- `GET /api/projects/{projectId}/files/{fileId}/collaborate?token={token}` — WebSocket connection for Yjs CRDT synchronization.

### Compilation
- `POST /api/projects/{projectId}/compile` — Enqueue asynchronous compile job.
- `GET /api/projects/{projectId}/compile/status` — Get status of latest compile job.
- `GET /api/projects/{projectId}/compile/pdf` — Get signed URL for compiled PDF.
- `GET /api/projects/{projectId}/compile/log` — Fetch raw LaTeX build log.
- `POST /api/internal/compile/callback` — Webhook for compiler completion callback.

### Project Members
- `GET /api/projects/{projectId}/members` — List collaborators and roles.
- `POST /api/projects/{projectId}/members` — Invite or update member role.
- `DELETE /api/projects/{projectId}/members/{userId}` — Remove collaborator.

---

## 🛠️ Local Development

### Prerequisites
- **Go 1.23+**
- **PostgreSQL** (or a free [Neon](https://neon.tech) serverless database)
- **GCP credentials** (if testing Cloud Tasks & GCS compilation locally)

### Configuration

Copy `.env.example` to `.env`:
```bash
cp .env.example .env
```

Set the following variables:
```ini
DATABASE_URL="postgresql://user:password@host/neondb?sslmode=require"
PORT=8080
ENV=development
ALLOWED_ORIGINS="http://localhost:5173,http://localhost:4173"
GCS_BUCKET="openleaf-artifacts-your-project"
TASKS_QUEUE="projects/your-project/locations/asia-southeast1/queues/latex-compiles"
COMPILER_URL="https://your-compiler-service.run.app"
BACKEND_URL="http://localhost:8080"
```

### Run Tests
```bash
go test -v ./...
```

### Start Server
```bash
go run ./cmd/server
```

---

## 🚀 Production Deployment

The backend is packaged as a multi-stage Alpine Docker container and deployed to Google Cloud Run:

```bash
gcloud run deploy openleaf-backend \
  --source . \
  --region asia-southeast1 \
  --allow-unauthenticated
```

---

## 📄 License

This project is licensed under the [Apache License, Version 2.0](LICENSE).
