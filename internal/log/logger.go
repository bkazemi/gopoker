package log

import (
	"os"
	"runtime"
	"strings"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

func init() {
	zerolog.CallerMarshalFunc = func(pc uintptr, _ string, _ int) string {
		fn := runtime.FuncForPC(pc).Name()
		if i := strings.LastIndex(fn, "/"); i >= 0 {
			fn = fn[i+1:]
		}
		return fn
	}

	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	zlog.Logger = zerolog.New(
		zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "2006/01/02 15:04:05"},
	).With().
		Timestamp().
		Caller().
		Logger()
}
