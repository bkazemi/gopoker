package main

import (
	"bytes"
	"encoding/gob"
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

func runClient(addr string, name string, isGUI bool) (err error) {
  if !strings.HasPrefix(addr, "ws://") {
    if strings.HasPrefix(addr, "http://") {
      addr = addr[7:]
    } else if strings.HasPrefix(addr, "https://") {
      addr = addr[8:]
    }

    addr = "ws://" + addr
  }

  fmt.Fprintf(os.Stderr, "connecting to %s ...\n", addr)
  conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
  if err != nil {
    return err
  }

  go func() {
    ticker := time.NewTicker(20 * time.Minute)

    client := &http.Client{}

    req, err := http.NewRequest("GET", "http://"+addr[5:], nil)
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

    sendData(&NetData{Request: NetDataClientExited}, conn)

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
  if isGUI {
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

  fmt.Fprintf(os.Stderr, "connected to %s\n", addr)

  go func() {
    defer recoverFunc()

    sendData(&NetData{Request: NetDataNewConn,
                      ClientSettings: &ClientSettings{Name: name}}, conn)

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
      dec.Decode(&netData)
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
        sendData(netData, conn)
      }
    }
  }()

  if err := frontEnd.Run(); err != nil {
    return err
  }

  return nil
}

func runGame(opts *options) (err error) {
  if opts.serverMode != "" {
    deck := &Deck{}
    if err := deck.Init(); err != nil {
      return err
    }

    table := &Table{NumSeats: opts.numSeats}
    if err := table.Init(deck, make([]bool, opts.numSeats)); err != nil {
      return err
    }

    randSeed()
    deck.Shuffle()

    server := &Server{}
    if err := server.Init(table, "0.0.0.0:"+opts.serverMode); err != nil {
      return err
    }

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
  } else if opts.connect != "" { // client mode
    if err := runClient(opts.connect, opts.name, opts.gui); err != nil {
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
  serverMode string
  connect    string
  name       string
  gui        bool
  numSeats   uint
}

/*
  TODO: - check if bets always have to be a multiple of blind(s)?
        - wrap errors
        - NetData related stuff is inefficient
        - add table password option

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

  var (
    serverMode string
    connect    string
    name       string
    gui        bool
    numSeats   uint
  )

  flag.Usage = func() {
    fmt.Println(usage)
    flag.PrintDefaults()
  }

  flag.StringVar(&serverMode, "s", "", "host a poker table on <port>")
  flag.StringVar(&connect, "c", "", "connect to a gopoker table")
  flag.StringVar(&name, "n", "", "name you wish to be identified by while connected")
  flag.BoolVar(&gui, "g", false, "run with a GUI")
  flag.UintVar(&numSeats, "ns", 7, "max number of players allowed at the table")
  flag.Parse()

  opts := &options{
    serverMode: serverMode,
    connect:    connect,
    name:       name,
    gui:        gui,
    numSeats:   numSeats,
  }

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
