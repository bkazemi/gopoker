package poker

import (
	"encoding/binary"
	"errors"

	crypto_rand "crypto/rand"
	math_rand "math/rand"

	"github.com/rivo/uniseg"
)

func Assert(cond bool, msg string) {
  if !cond {
    panic(msg)
  }
}

func AbsUInt64(x, y uint64) uint64 {
  if x > y {
    return x - y
  }

  return y - x
}

func AbsChips(x, y Chips) Chips {
  if x > y {
    return x - y
  }

  return y - x
}

func MinUInt64(x, y uint64) uint64 {
  if x < y {
    return x
  }

  return y
}

func MinChips(x, y Chips) Chips {
  if x < y {
    return x
  }

  return y
}

func MaxUInt64(x, y uint64) uint64 {
  if (x > y) {
    return x
  }

  return y
}

func MaxChips(x, y Chips) Chips {
  if (x > y) {
    return x
  }

  return y
}

func MaxInt(x, y int) int {
  if (x > y) {
    return x
  }

  return y
}

func PlayerMapToArr(playerMap map[string]*Player) []*Player {
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
}

func (p *Panic) panic(msg string) {
  p.panicked = true
  panic(msg)
}

func (p *Panic) IfNoPanic(deferredFunc func()) {
  if !p.panicked {
    deferredFunc()
  }
}

func PanicRetToError(err interface{}) error {
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

func RandSeed() {
  var b [8]byte

  _, err := crypto_rand.Read(b[:])
  if err != nil {
    panic("randSeed(): problem with crypto/rand")
  }

  math_rand.Seed(int64(binary.LittleEndian.Uint64(b[:])))
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func RandString(n int) string {
  b := make([]rune, n)

  for i := range b {
    b[i] = letters[math_rand.Intn(len(letters))]
  }

  RandSeed() // re-seed just in case

  return string(b)
}

// FillLeft return string filled in left by spaces in w cells
//
// taken from github.com/go-runewidth
func FillLeft(s string, w int) string {
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
func FillRight(s string, w int) string {
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
