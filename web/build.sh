#!/bin/sh

GOOS=js GOARCH=wasm go build -o gopoker.wasm github.com/bkazemi/gopoker
