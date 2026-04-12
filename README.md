![gopoker](assets/logo.gif)
# Poker game in Go (WIP)

### Try it [here](https://poker.shirkadeh.org)

## Setup
```sh
$ git clone https://github.com/bkazemi/gopoker
$ cd gopoker

# build the server (backend requirement: Go 1.26)
$ go build -o gopoker ./cmd/gopoker

# install web frontend dependencies (web requirement: Node.js + Yarn)
$ cd web/
$ yarn install
```

## Running Locally
```sh
# terminal 1: start the poker server on localhost:7777
$ ./gopoker -s 7777

# terminal 2: start the Next.js frontend on localhost:3000
# this step is web-only and requires Node.js + Yarn
$ cd web/
$ export NEXT_PUBLIC_GOPOKER_SERVER_ADDR='localhost:7777'
$ yarn dev
```

```sh
# create a new room directly on the Go server
$ curl \
  -H "Content-Type: application/json" \
  -d '{"roomName":"test","numSeats":6,"lock":0,"password":""}' \
  http://localhost:7777/new

# connect to that room as a CLI client
$ ./gopoker -c ws://localhost:7777/room/test/cli -n alice
```

For a production-style frontend run, use `yarn build && yarn start` in `web/`.

## Configuration
The web app reads these environment variables:

- `NEXT_PUBLIC_GOPOKER_SERVER_ADDR`: backend host:port, used when explicit HTTP/WS URLs are not set. Defaults to `localhost`.
- `NEXT_PUBLIC_GOPOKER_SERVER_HTTPURL`: full backend HTTP base URL, for example `http://localhost:7777`.
- `NEXT_PUBLIC_GOPOKER_SERVER_WSURL`: full backend WebSocket base URL, for example `ws://localhost:7777`.
- `NEXT_PUBLIC_SSL_ENABLED`: set to `true` to derive `https://` and `wss://` URLs from `NEXT_PUBLIC_GOPOKER_SERVER_ADDR`.
- `NEXT_PUBLIC_SHOW_LOG`: when unset, browser logging is muted by default. Set it to keep logs visible.

## CLI Usage
The Go binary supports these main flags:

- `-s <port>`: run the poker server.
- `-c <ws-url>`: connect as a CLI client.
- `-n <name>`: set the player name for a CLI connection.
- `-pass <password>`: send a room password when joining.
- `-S`: join as a spectator.
- `-ns <count>`: max number of players allowed at the table (default 7).
- `-g`: reserved GUI mode flag.

## HTTP API
The Go server exposes:

- `GET /health`: liveness check.
- `GET /status`: returns `{"status":"running"}`.
- `POST /new`: create a room. JSON fields: `roomName`, `numSeats`, `lock`, `password`.
- `GET /roomCount`: returns the number of active rooms.
- `GET /rooms`: returns room metadata for the room list UI.
- `GET /room/{roomName}`: returns room availability status.
- `GET /room/{roomName}/{connType}`: WebSocket endpoint for `cli` or `web` clients.

`POST /new` returns JSON containing `URL`, `roomName`, and `creatorToken`.

For `lock`, use:

- `0`: no lock
- `1`: player lock
- `2`: spectator lock
- `3`: player and spectator lock

## Pre-commit
```sh
$ pre-commit install
```

The repo includes a `pre-commit` hook for `gofmt`, which formats staged `*.go` files before commit.
