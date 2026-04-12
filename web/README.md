# gopoker web

Next.js 14 (pages router) + React 18 frontend for gopoker. Talks to the Go server over HTTP (room creation, metadata) and WebSocket (gameplay), using msgpack for the wire format.

## Setup
```sh
$ yarn install
```

Requires Node.js and Yarn. The Go server must be running separately — see the top-level README.

## Development
```sh
# point at a running Go server
$ export NEXT_PUBLIC_GOPOKER_SERVER_ADDR='localhost:7777'
$ yarn dev      # http://localhost:3000
```

Other scripts:
- `yarn build` — production build
- `yarn start` — serve the production build
- `yarn lint` — next lint

## Environment
Resolved in `serverConfig.js`:

- `NEXT_PUBLIC_GOPOKER_SERVER_ADDR` — backend `host:port`. Default `localhost`.
- `NEXT_PUBLIC_GOPOKER_SERVER_HTTPURL` — full HTTP base URL; overrides the derived value.
- `NEXT_PUBLIC_GOPOKER_SERVER_WSURL` — full WebSocket base URL; overrides the derived value.
- `NEXT_PUBLIC_SSL_ENABLED` — `true` to derive `https://` / `wss://` from `…_ADDR`.
- `NEXT_PUBLIC_SHOW_LOG` — when unset, browser logging is muted.

If `HTTPURL` / `WSURL` are not set, they're derived from `ADDR` + `SSL_ENABLED`.

## Layout
```
pages/
  index.jsx           landing / room creation
  rooms.jsx           room list
  room/[roomID].jsx   table view (WebSocket gameplay)
  api/                Next.js API routes that proxy the Go server
    new.js            POST /new
    check/            room availability
    roomCount.js, roomList.js, status.js
components/           Game, Player, TableCenter, Chat, Header, modals, …
lib/
  libgopoker.js       WS client, msgpack framing, game-state reducer
  useDeferredLoading.js, useFlickSpin.js
GameContext.jsx       React context for shared game state
serverConfig.js       env-var resolution
styles/               CSS modules
```

## Server interaction
- HTTP requests go through `pages/api/*`, which forward to the Go server so the browser only talks to the Next.js origin.
- Gameplay uses a WebSocket to `/room/{roomName}/web` on the Go server (direct, not proxied). See `lib/libgopoker.js` for framing and message handling.
- Payloads are msgpack-encoded (`@msgpack/msgpack`).

See the top-level README for the Go server's HTTP API and flags.
