package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type FrontEnd interface {
  InputChan() chan *NetData
  OutputChan() chan *NetData
  Init() error
  Run() error
  Finish() chan error
  Error() chan error
}

func runClient(opts options) (err error) {
  if !strings.HasPrefix(opts.addr, "ws://") {
    if strings.HasPrefix(opts.addr, "http://") {
      opts.addr = opts.addr[7:]
    } else if strings.HasPrefix(opts.addr, "https://") {
     opts.addr = opts.addr[8:]
    }

    opts.addr = "ws://" + opts.addr
  }

  fmt.Fprintf(os.Stderr, "connecting to %s ...\n", opts.addr)
  conn, _, err := websocket.DefaultDialer.Dial(opts.addr, nil)
  if err != nil {
    return err
  }

  go func() {
    ticker := time.NewTicker(20 * time.Minute)

    client := &http.Client{}

    req, err := http.NewRequest("GET", "http://"+opts.addr[5:], nil)
    if err != nil {
      fmt.Fprintf(os.Stderr, "problem setting up keepalive request %s\n",
                  err.Error())

      return
    }
    req.Header.Add("keepalive", "true")

    for {
      <-ticker.C

      _, err := client.Do(req)
      if err != nil {
        fmt.Fprintf(os.Stderr, "problem sending a keepalive request %s\n",
                    err.Error())

        return
      }
    }
  }()

  defer func() {
    fmt.Fprintf(os.Stderr, "closing connection\n")

    (&NetData{
      Request: NetDataClientExited,
      Client: &Client{conn: conn, connType: "cli"},
    }).Send()

    err := conn.WriteMessage(websocket.CloseMessage,
      websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
    if err != nil {
      fmt.Fprintf(os.Stderr, "write close err: %s\n", err.Error())
    }

    /*select {
      case <-time.After(time.Second * 3):
        fmt.Fprintf(os.Stderr, "timeout: couldn't close connection properly.\n")
      }*/

    return
  }()

  var frontEnd FrontEnd
  if opts.GUI {
    //frontEnd := runGUI()
  } else { // CLI mode
    frontEnd = &CLI{}

    if err := frontEnd.Init(); err != nil {
      return err
    }
  }

  recoverFunc := func() {
    if err := recover(); err != nil {
      if frontEnd != nil {
        frontEnd.Finish() <- panicRetToError(err)
      }
      fmt.Printf("recover() done\n")
    }
  }

  fmt.Fprintf(os.Stderr, "connected to %s\n", opts.addr)

  // listen for messages from the server and send them to the frontend
  go func() {
    defer recoverFunc()

    (&NetData{
      Request: NetDataNewConn,
      Client: &Client{
        Settings: &ClientSettings{Name: opts.name, Password: opts.pass},
        conn: conn,
        connType: "cli",
      },
    }).Send()

    for {
      _, data, err := conn.ReadMessage()

      if err != nil {
        if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
          frontEnd.Finish() <- err
        } else {
          frontEnd.Finish() <- nil // normal exit
        }

        return
      }

      netData := &NetData{}
      dec := gob.NewDecoder(bytes.NewReader(data))
      if err := dec.Decode(&netData); err != nil {
        fmt.Printf("runClient(): problem decoding gob stream %s\n", err.Error())

        frontEnd.Finish() <- errors.New("server had a problem decoding a gob stream")
        return
      }
      frontEnd.InputChan() <- netData

      /*var gobBuf bytes.Buffer
        enc := gob.NewEncoder(&gobBuf)

        enc.Encode(frontEnd.OutputChan())

        writeConn.Write(gobBuf.Bytes())
        writeConn.Flush()*/
    }
  }()

  // redirect CLI requests (+ input) to server
  go func() {
    for {
      select {
      case err := <-frontEnd.Error(): // error from front-end
        if err != nil {
          fmt.Fprintf(os.Stderr, "front-end err: %s\n", err.Error())
        }
        return
      case netData := <-frontEnd.OutputChan():
        netData.SendToConn(conn, "cli")
      }
    }
  }()

  if err := frontEnd.Run(); err != nil {
    return err
  }

  return nil
}

func runGame(opts options) (err error) {
  if opts.serverPort != "" {
    deck := NewDeck()

    table, tableErr := NewTable(deck, opts.numSeats, TableLockNone, "",
                                make([]bool, opts.numSeats))
    if tableErr != nil {
      return tableErr
    }

    randSeed()
    deck.Shuffle()

    server := NewServer("0.0.0.0:" + opts.serverPort)

    if err := server.run(); err != nil {
      return err
    }

    if false { // TODO: implement CLI only mode
      deck.Shuffle()
      table.Deal()
      table.DoFlop()
      table.DoTurn()
      table.DoRiver()
      table.PrintSortedCommunity()
      //table.BestHand()
    }
  } else if opts.addr != "" { // client mode
    if err := runClient(opts); err != nil {
      return err
    }
  } else { // offline game

  }

  /*if false {
    if err := gui_run(table); err != nil {
      fmt.Printf("gui_run() err: %v\n", err)
      return nil
    }
  }*/

  return nil
}

var printer *message.Printer

func init() {
  printer = message.NewPrinter(language.English)
}

type options struct {
  serverPort string
  addr       string
  name       string
  pass       string
  GUI        bool
  numSeats   uint8
}

/*
  TODO: - check if bets always have to be a multiple of blind(s)?
        - wrap errors
        - NetData related stuff is inefficient
        - check if webasm/JS stuff allows for separate packages now

        cli.go:
        - figure out why refocusing on a primitive increments the highlighted
          sub element
        - allow for 'k' (x1000) rune in bet field
*/
func main() {
  processName, err := os.Executable()
  if err != nil {
    processName = "gopoker"
  }

  usage := "usage: " + processName + " [options]"

  flag.Usage = func() {
    fmt.Println(usage)
    flag.PrintDefaults()
  }

  var (
    numSeats uint = 0
  )

  opts := options{}

  flag.StringVar(&opts.serverPort, "s", "", "host a poker table on <port>")
  flag.StringVar(&opts.addr, "c", "", "connect to a gopoker table")
  flag.StringVar(&opts.name, "n", "", "name you wish to be identified by while connected")
  flag.StringVar(&opts.pass, "pass", "", "login password (as client)")
  flag.BoolVar(&opts.GUI, "g", false, "run with a GUI")
  flag.UintVar(&numSeats, "ns", 7, "max number of players allowed at the table")
  flag.Parse()

  if numSeats > uint(^uint8(0)) {
    fmt.Printf("main(): numSeats %v is too large\n", numSeats)
    return
  }

  opts.numSeats = uint8(numSeats)

  /*go func() {
    fmt.Println("TMP: adding pprof server")
    runtime.SetMutexProfileFraction(5)
    fmt.Println(http.ListenAndServe("localhost:6060", nil))
  }()*/

  if err := runGame(opts); err != nil {
    fmt.Println(err)
    return
  }
}
