<p align="center">
  <img src="cmd/lanify-swdl/logo.svg" width="180" alt="lanify-swdl logo">
</p>

# lanify-swdl

`lanify-swdl` is a small Go service for browsing Steam Workshop pages through a local proxy and downloading Workshop items with SteamCMD.

It exposes Workshop pages under local routes, rewrites Steam asset URLs so the pages keep working through the proxy, adds download links for items and collections, and serves the downloaded content as zip archives.

## Features

- Local reverse proxy for Steam Workshop, app, public, and shared file pages.
- URL rewriting for Steam Community, Steam static assets, media, video, and user content hosts.
- Download buttons injected into proxied Workshop item and collection pages.
- SteamCMD-backed downloads for single Workshop items and collections.
- Cache validation using metadata files so unchanged content can be reused.
- Zip packaging for item downloads and collection downloads.
- Structured logging with Go's standard `log/slog`.
- GoReleaser configuration for Windows and Linux builds.

## Requirements

- Go 1.25 or newer.
- Network access to Steam Community, Steam CDN hosts, and the SteamCMD installer.
- SteamCMD, either installed by this service or already available at `-steamcmdpath`.

Some Workshop content can be downloaded anonymously. Other content may require a Steam account, app ownership, or access to private/unlisted Workshop items. This project does not bypass Steam permissions.

## Build

```sh
go build -trimpath -o lanify-swdl ./cmd/lanify-swdl
```

On Windows:

```powershell
go build -trimpath -o lanify-swdl.exe ./cmd/lanify-swdl
```

## Run

```sh
./lanify-swdl -listenhost 0.0.0.0 -listenport 8080
```

Then open:

```text
http://localhost:8080/workshop/
```

By default, the service installs SteamCMD if it is missing and logs in to SteamCMD anonymously. To use a Steam account for downloads:

```sh
./lanify-swdl -steamuser your_username -steampassword your_password
```

Avoid passing credentials on shared machines where shell history or process listings may be visible.

## Command-Line Options

| Flag               | Default    | Description                                                              |
|--------------------|------------|--------------------------------------------------------------------------|
| `-listenhost`      | `0.0.0.0`  | Host or IP address for the HTTP server.                                  |
| `-listenport`      | `8080`     | HTTP server port.                                                        |
| `-steamcmdpath`    | `steamcmd` | SteamCMD install directory. On Linux, the default resolves to `~/Steam`. |
| `-installsteamcmd` | `true`     | Install SteamCMD on startup when the executable is missing.              |
| `-debug`           | `false`    | Enable debug-level logging.                                              |
| `-steamuser`       | empty      | Steam username. Empty uses anonymous SteamCMD login.                     |
| `-steampassword`   | empty      | Steam password.                                                          |

## Routes

| Route                | Upstream                                       |
|----------------------|------------------------------------------------|
| `/`                  | Redirects xto `/workshop/`                     |
| `/workshop/*path`    | `https://steamcommunity.com/workshop/*path`    |
| `/app/*path`         | `https://steamcommunity.com/app/*path`         |
| `/public/*path`      | `https://steamcommunity.com/public/*path`      |
| `/sharedfiles/*path` | `https://steamcommunity.com/sharedfiles/*path` |

Additional Steam hosts are proxied through internal routes such as `/steamstatic-community`, `/steamstatic-cdn`, `/steamstore`, `/steam-media`, `/steam-video`, and `/steamusercontent-images`.

Steam login, registration, account, profile, and identity routes are blocked by the proxy. If downloads need account access, pass SteamCMD credentials with `-steamuser` and `-steampassword`.

## Download API

| Endpoint                                     | Description                                                    |
|----------------------------------------------|----------------------------------------------------------------|
| `GET /api/workshop/:app_id/:workshop_id`     | Downloads or reuses one Workshop item and returns a zip file.  |
| `GET /api/collection/:app_id/:collection_id` | Downloads or reuses collection items and returns one zip file. |

Examples:

```text
http://localhost:8080/api/workshop/4000/123456789
http://localhost:8080/api/collection/4000/987654321
```

The `app_id` must be the Steam app ID that owns the Workshop content.

## Cache and Output

SteamCMD content is stored under:

```text
<steamcmdpath>/steamapps/workshop/content/<app_id>/<workshop_id>
```

Generated archives are stored under:

```text
<steamcmdpath>/downloads
```

Each cached item also gets a `.meta` file containing the app ID, Workshop ID, resolved name, file count, total size, validation hash, and validation timestamp. The metadata is used to decide whether a cached item can be reused or should be downloaded again.

## Development

Run the test suite:

```sh
go test ./...
```

Format the code:

```sh
gofmt -w ./cmd ./internal
```

Package layout:

```text
cmd/lanify-swdl      server entrypoint, route matching, and embedded logo
internal/handler     HTTP handlers, proxy rewriting, and download endpoints
internal/steam       Steam Community scraping helpers
internal/steamcmd    SteamCMD install, sync, validation, and cache logic
internal/fileutil    archive, metadata, path, download, and validation helpers
```

## Release Builds

GoReleaser is configured to build `amd64` and `arm64` binaries for Windows and Linux.

Snapshot build:

```sh
goreleaser build --snapshot --clean
```

Release:

```sh
goreleaser release --clean
```

## Scope

`lanify-swdl` is a local helper around Steam Workshop pages and SteamCMD downloads. It does not remove Steam access checks, bypass app ownership requirements, or provide access to Workshop content that the Steam account cannot download.
