package main

import (
	"encoding/binary"
	"errors"

	crypto_rand "crypto/rand"
	math_rand "math/rand"
)

func assert(cond bool, msg string) {
  if !cond {
    panic(msg)
  }
}

func absUInt64(x, y uint64) uint64 {
  if x > y {
    return x - y
  }

  return y - x
}

func absChips(x, y Chips) Chips {
  if x > y {
    return x - y
  }

  return y - x
}

func minUInt64(x, y uint64) uint64 {
  if x < y {
    return x
  }

  return y
}

func minChips(x, y Chips) Chips {
  if x < y {
    return x
  }

  return y
}

func maxUInt64(x, y uint64) uint64 {
  if (x > y) {
    return x
  }

  return y
}

func maxChips(x, y Chips) Chips {
  if (x > y) {
    return x
  }

  return y
}

// used to avoid execution of defers after a panic()
type Panic struct {
  panicked  bool
  panic     func(string)
  ifNoPanic func(func())
}

func (p *Panic) Init() {
  p.panicked = false

  p.panic = func(msg string) {
    p.panicked = true
    panic(msg)
  }

  p.ifNoPanic = func(deferredFunc func()) {
    if !p.panicked {
      deferredFunc()
    }
  }
}

func panicRetToError(err interface{}) error {
  var typedErr error

  switch errType := err.(type) {
  case string:
    typedErr = errors.New(errType)
  case error:
    typedErr = errType
  default:
    typedErr = errors.New("unknown panic")
  }

  return typedErr
}

func randSeed() {
  var b [8]byte

  _, err := crypto_rand.Read(b[:])
  if err != nil {
    panic("randSeed(): problem with crypto/rand")
  }

  math_rand.Seed(int64(binary.LittleEndian.Uint64(b[:])))
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func randString(n int) string {
  b := make([]rune, n)

  for i := range b {
    b[i] = letters[math_rand.Intn(len(letters))]
  }

  randSeed() // re-seed just in case

  return string(b)
}
