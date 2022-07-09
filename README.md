# gopoker
## A poker server & client in Go (WIP)
### *still being heavily modified. clone at your own risk.*
### *currently working in the [websockets](../../tree/websockets) branch.*
```sh
git clone https://github.com/bkazemi/gopoker
cd gopoker
git checkout websockets
go build
```

```sh
# *nix
./gopoker -s 777 # create a new poker server on localhost:777
./gopoker -c ws://localhost:777/cli # connect to the server as a CLI client
```

```cmd
C:\[whateverdirs]\gopoker\"gopoker.exe" REM Windows CMD
```
