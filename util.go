package main

import (
	"encoding/binary"
	"errors"

	crypto_rand "crypto/rand"
	math_rand "math/rand"

	"github.com/rivo/uniseg"
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

func maxInt(x, y int) int {
  if (x > y) {
    return x
  }

  return y
}

func playerMapToArr(playerMap map[string]*Player) []*Player {
  if playerMap == nil || len(playerMap) == 0 {
    return []*Player{}
  }

  arr := make([]*Player, 0)

  for _, p := range playerMap {
    arr = append(arr, p)
  }

  return arr
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

// FillLeft return string filled in left by spaces in w cells
//
// taken from github.com/go-runewidth
func fillLeft(s string, w int) string {
  width := uniseg.StringWidth(s)
  count := w - width

  if count > 0 {
    b := make([]byte, count)
    for i := range b {
      b[i] = ' '
    }
    return string(b) + s
  }

  return s
}

// FillRight return string filled in right by spaces in w cells
//
// taken from github.com/go-runewidth
func fillRight(s string, w int) string {
  width := uniseg.StringWidth(s)
  count := w - width

  if count > 0 {
    b := make([]byte, count)
    for i := range b {
      b[i] = ' '
    }
    return s + string(b)
  }

  return s
}
