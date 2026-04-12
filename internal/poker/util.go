package poker

import (
	"errors"
	"math/rand"

	"github.com/rivo/uniseg"
)

func Assert(cond bool, msg string) {
	if !cond {
		panic(msg)
	}
}

// used to avoid execution of defers after a panic()
type Panic struct {
	panicked bool
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

func PanicRetToError(err any) error {
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

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func RandString(n int) string {
	b := make([]rune, n)

	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

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
