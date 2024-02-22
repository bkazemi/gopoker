![gopoker](https://github.com/bkazemi/gopoker/blob/web/assets/logo.gif)
# Poker game in Go (WIP)

### Try it [here](https://poker.shirkadeh.org)

### Development currently active on [web](https://github.com/bkazemi/gopoker/tree/web) branch

## Setup
```sh
$ git clone https://github.com/bkazemi/gopoker
$ cd gopoker
$ git checkout web

# build the server
$ go build

# build the web frontend
$ cd web/
$ yarn build
```

```sh
# start the poker server on localhost:777
$ gopoker -s 777

# create a new poker room
$ curl -d '{"roomName": "test"}' -H "Content-Type: application/json" http://localhost:777/new

# connect to the poker room as a CLI client
$ gopoker -c ws://localhost:777/cli

# alternatively,  connect to the server as a web client
$ export  NEXT_PUBLIC_GOPOKER_SERVER_ADDR='localhost:777' # tell web frontend where the server is
$ yarn start # start next.js web frontend
```
