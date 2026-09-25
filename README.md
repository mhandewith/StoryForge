# StoryForge

A minimal Go backend packaged for Unraid. Database, frontend, and audio recording follow in later milestones.

## Publish to GitHub Container Registry

This uses the same deployment approach as SagesSagaTracker: GitHub builds and publishes the image, and Unraid downloads it.

1. Commit and push the project, including `.github/workflows/publish-image.yml`, to `main`.
2. Open [GitHub Actions](https://github.com/mhandewith/StoryForge/actions) and wait for **Publish Docker image** to succeed.
3. Open your GitHub account's **Packages**, select **storyforge**, then **Package settings**. For downloads without credentials, change visibility to **Public**. This makes the image downloadable by anyone; source repository visibility is separate. New packages default to private. Alternatively, keep it private and run `docker login ghcr.io -u mhandewith` on Unraid, supplying a GitHub classic token with `read:packages` at the password prompt.

The workflow builds Linux amd64, runs Go tests, starts the container, verifies its health, and publishes:

- `ghcr.io/mhandewith/storyforge:latest`
- `ghcr.io/mhandewith/storyforge:<full-commit-sha>`

Pull requests build and test without publishing. Manual runs on main also publish. The built-in GitHub token handles publishing; no Docker Hub account or custom secret is required. GitHub Actions and package publishing must be allowed by repository policy.

**The image address only works after the first successful publish and package access setup.** Preparing the workflow locally does not publish it.

See [GitHub publishing documentation](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images) and [registry access](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry).

## Install on Unraid

Go to **Docker > Add Container**, start without a template, and enter:

| Setting | Value |
| --- | --- |
| Name | `storyforge` |
| Repository | `ghcr.io/mhandewith/storyforge:latest` |
| Network Type | `Bridge` |
| Console shell, if shown | `sh` |
| WebUI, Advanced View | `http://[IP]:[PORT:8080]/` |

Click **Add another Path, Port, Variable, Label or Device**, choose **Port**, and set:

| Setting | Value |
| --- | --- |
| Name | `HTTP` |
| Container Port | `8080` |
| Host Port | `8088`, or another unused port |
| Connection Type | `TCP` |

Click **Apply**. Unraid downloads the image and starts the backend. Enable **Autostart** if desired. No source copying, server build, volume mappings, privileged mode, or Compose plugin is required.

See [Unraid container settings](https://docs.unraid.net/unraid-os/using-unraid-to/run-docker-containers/managing-and-customizing-containers/).

## Test it

Open `http://YOUR-UNRAID-IP:8088/healthz` from a device on your LAN:

```json
{"service":"StoryForge","status":"ok"}
```

The root URL `http://YOUR-UNRAID-IP:8088/` returns a startup message.
In Unraid's terminal:

```sh
curl -i http://127.0.0.1:8088/healthz
docker inspect --format='{{.State.Health.Status}}' storyforge
docker logs --tail=50 storyforge
```

Expect HTTP 200 and `healthy` within about 30 seconds. Restart the container and refresh the URL to confirm it comes back. Health checks only the HTTP service; there are no database or storage dependencies yet.

## Updates and troubleshooting

- **Update:** push changes to main, wait for publishing to succeed, then use Unraid's **Check for Updates / Update** to download and recreate the container. Restart alone does not fetch a new image.
- **Denied/unauthorized:** check package visibility or registry login.
- **Manifest unknown:** check the first workflow succeeded and the Repository field is exactly `ghcr.io/mhandewith/storyforge:latest`.
- **Port allocated:** change host port 8088 to an unused port, leave container port 8080, and update your browser URL.
- **Container exits/unhealthy:** inspect `docker logs storyforge` and `docker inspect storyforge`.
- **Unraid curl works but browser fails:** check server IP, host port, and that your device is on the same LAN rather than an isolated guest network.

## Local development

Requires Go 1.24 or newer; Docker builds use Go 1.26.

```powershell
cd C:\Repos\StoryForge\backend
go test ./...
go run ./cmd/server
```

Open <http://localhost:8080/healthz>. Stop with Ctrl+C.
The optional `PORT` environment variable changes the default 8080 listener.
Unknown URLs return 404.

## Docker Compose alternative

To pull the published image, from the repository root:

```sh
docker compose pull
docker compose up -d
```

To build local source for development:

```sh
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build
```

Both expose <http://localhost:8088/>. Put `STORYFORGE_PORT=8090` in a local `.env` file to change the host port. Stop with `docker compose down`. Use either Compose or the Unraid template to manage the container, not both.

This milestone has no persistent data, authentication, HTTPS, or frontend.
