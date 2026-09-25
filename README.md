# StoryForge

A collaborative script and voice studio for the family. Milestone 2 provides
persistent project and script management. Recording and audio processing come next.

Deployed home-network address: **http://192.168.86.127:8088**.

## What works

- Create projects and ordered scenes.
- Create reusable actors (Hazel, Hannah, Dad) and characters within each project.
- Assign an actor to each character.
- Create and edit ordered dialogue, performance directions, and start times.
- Reload saved data and retain it across application/database restarts.
- Preserve dialogue revisions for future recordings. Conflicting edits return a
  conflict instead of silently overwriting another editor's work.

The React/TypeScript/Vite admin is served by the Go backend at the same address.
PostgreSQL stores metadata. There is no recording, authentication, or audio storage
in this milestone. The app is intended for the home network.

## Upgrade the existing Unraid deployment

**Configure PostgreSQL and the connection variables before updating StoryForge.**
This version requires a database. Keep the same HTTP mapping (host 8088 to
container 8080). No changes to your router are required.

### 1. Create a private Docker network

In Unraid's web terminal, run once:

```sh
docker network create storyforge-net
```

If Docker says the network already exists, reuse it. This lets the containers
reach one another by name without publishing PostgreSQL's port to your LAN.

### 2. Add PostgreSQL through Docker > Add Container

| Field | Value |
| --- | --- |
| Name | `storyforge-db` |
| Repository | `postgres:17-alpine` |
| Network Type | `Custom: storyforge-net` |
| Privileged | Off |

Add these **Variables** (key and value):

| Key | Value |
| --- | --- |
| `POSTGRES_USER` | `storyforge` |
| `POSTGRES_DB` | `storyforge` |
| `POSTGRES_PASSWORD` | Choose a unique password; use the same value in StoryForge below |

Add one **Path**:

| Container path | Host path | Access |
| --- | --- | --- |
| `/var/lib/postgresql/data` | `/mnt/user/appdata/storyforge/postgres` | Read/Write |

Use a new empty directory dedicated to this database. No port mapping is needed.
Click **Apply**. Enable **Autostart** and check the log for
`database system is ready to accept connections`.

PostgreSQL initialization variables only create the account/database on the
first launch with empty storage. Changing the password variable later does not
change an existing database password. Keep the image on major version 17;
major-version upgrades need a database migration.

### 3. Update the existing StoryForge container settings

Edit StoryForge in Unraid:

| Field | Value |
| --- | --- |
| Repository | `ghcr.io/mhandewith/storyforge:latest` |
| Network Type | `Custom: storyforge-net` |
| WebUI, Advanced View | `http://[IP]:[PORT:8080]/` |

Add these **Variables**:

| Key | Value |
| --- | --- |
| `PGHOST` | `storyforge-db` |
| `PGPORT` | `5432` |
| `PGUSER` | `storyforge` |
| `PGDATABASE` | `storyforge` |
| `PGPASSWORD` | The same password chosen above |
| `PGSSLMODE` | `disable` |

Retain the TCP port mapping **host 8088 → container 8080**. Click **Apply**, then
use **Check for Updates / Update** (or **Force Update**) to pull the published
image. The app waits up to 60 seconds for PostgreSQL and applies migrations
automatically. Enable Autostart, with PostgreSQL before StoryForge.

No app volume is required yet. All current records are in PostgreSQL's mapped
directory. Do not delete that directory when recreating containers.

### 4. Acceptance test

1. Open **http://192.168.86.127:8088**.
2. Create a project, such as **The lantern in the woods**.
3. Open **Cast & characters**. Add Hazel and Hannah as actors.
4. Add their characters and choose the actor for each.
5. Open **Scenes & script** and add one scene.
6. Add ten dialogue lines, five per character. Positions control reading order.
   Start time is in milliseconds and can remain 0 while drafting; equal times
   are allowed for future overlapping dialogue.
7. Edit one line and click **Save changes**.
8. Reload the page and select the project again. Confirm the cast and lines.
9. Stop StoryForge, restart PostgreSQL, then start StoryForge. Confirm all records
   remain. This also checks that migrations do not duplicate data.

Status URLs:

- [Process health](http://192.168.86.127:8088/healthz)
- [Database readiness](http://192.168.86.127:8088/readyz)

The Docker health check uses readiness, which requires PostgreSQL.
The process health endpoint remains available during a later database outage.

### Troubleshooting

- **Container exits:** check `docker logs storyforge`. Missing connection
  configuration and startup database timeouts produce explicit messages.
- **Database unavailable:** confirm both containers use `storyforge-net`, check
  `PGHOST`, credentials, and `docker logs storyforge-db`.
- **Denied pulling image:** the GitHub package must be public or Unraid must be
  signed in to GHCR. See the publishing section.
- **Port allocated:** use another unused host port, leaving container port 8080.
- **Position in use:** choose an unused positive position in that scene. Positions
  may have gaps; moving a line never silently replaces another line.
- **Line changed:** another edit won the race. Copy any unsaved text, use Reload,
  and edit the current version.
- **Database password changed only in Unraid:** restore the original connection
  password or explicitly change the database role password using PostgreSQL.

Back up the database to an existing backup share (adjust the destination):

```sh
docker exec storyforge-db pg_dump -U storyforge -d storyforge -Fc > /mnt/user/backups/storyforge.dump
```

This writes a database dump on the host. Back up using PostgreSQL tools rather
than copying database files while PostgreSQL is running.

## Local development / Docker Compose

Copy `.env.example` to `.env` and replace the example password.
With Docker in Linux-container mode:

```sh
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build
```

Open <http://localhost:8088>. PostgreSQL uses a named Docker volume and is not
published to a host port. `docker compose down` retains it; `down -v` deletes it.
To run published images instead:

```sh
docker compose pull
docker compose up -d
```

For development outside Docker, use Go 1.25+ and Node 22.12+ (CI uses Go 1.26 and
Node 22). Run `npm ci` and `npm run build` in `frontend`. Configure
`PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE`, and `PGSSLMODE`
for a reachable local PostgreSQL database, then run `go run ./cmd/server` from
`backend`. Alternatively set `DATABASE_URL` to a PostgreSQL connection URL
(URL-encode special characters in credentials). The separate variables avoid
URL-encoding passwords.

The server defaults to port 8080 and `WEB_DIR=../frontend/dist`. For frontend
hot reload, run `npm run dev` in `frontend`; Vite proxies API requests to
the backend on 8080.

## Architecture and API

- `backend/cmd/server`: HTTP lifecycle and static frontend serving.
- `backend/internal/api`: JSON API, validation, and PostgreSQL queries.
- `backend/migrations`: numbered SQL migrations embedded in the Go binary;
  applied once inside a transaction with a database advisory lock.
- `frontend/src`: React admin screens.
- `scripts/container-smoke.py`: isolated database and container tests.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/workspace` | Consistent snapshot of projects, cast, scenes, and dialogue |
| POST | `/api/projects` | Create project: `name` |
| POST | `/api/actors` | Create actor: `name` |
| POST | `/api/characters` | Create character: `project_id, name` |
| POST | `/api/scenes` | Create scene: `project_id, name, position` |
| PUT | `/api/assignments/{character_id}` | Assign/reassign role: `actor_id` |
| POST | `/api/events` | Create dialogue |
| PUT | `/api/events/{id}` | Edit dialogue with current `revision` |
| GET | `/healthz` | Process liveness |
| GET | `/readyz` | Database readiness |

Dialogue fields: `project_id, scene_id, character_id, text, direction, position,
start_ms`. Edits also require `revision`. Relationships are enforced by foreign
keys, including composite keys preventing cross-project scene/character mixing.
The database archives every dialogue revision. There are no delete endpoints yet.

## Tests and publishing

Pushes to `main` run Go tests/vet, build the image, exercise the PostgreSQL API,
run a Chromium browser test that creates Hazel and Hannah's ten-line scene,
verify data after recreating the app and restarting the database, and then
publish to GHCR. Browser screenshots/traces are saved as workflow artifacts.
Pull requests run the same checks without publishing. Failures block publication.

```text
ghcr.io/mhandewith/storyforge:latest
ghcr.io/mhandewith/storyforge:<full-commit-sha>
```

Use [GitHub Actions](https://github.com/mhandewith/StoryForge/actions) to check a
build. The workflow uses GitHub's built-in token. Public package visibility
allows Unraid downloads without credentials. For a private package, use
`docker login ghcr.io -u mhandewith` and a classic token with `read:packages`.

This milestone uses one actor per character and manual line/scene positions.
It does not yet provide actor recording, scene rendering, audio assets, bulk
script import, or project/actor/character renaming.

Official references:
[Unraid container settings](https://docs.unraid.net/unraid-os/using-unraid-to/run-docker-containers/managing-and-customizing-containers/),
[PostgreSQL image](https://hub.docker.com/_/postgres),
[GitHub registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry).
