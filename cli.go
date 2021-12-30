package main

import (
  "fmt"
  "strconv"

  "github.com/gdamore/tcell/v2"
  "github.com/rivo/tview"
)

type CLI struct {
  app               *tview.Application
  pages             *tview.Pages
  gameGrid          *tview.Grid
 
  commView,
  otherPlayersView,
  actionsView,
  holeView           *tview.TextView

  otherPlayersFlex   *tview.Flex
  otherPlayersFlexMap map[string]*tview.TextView

  actionsForm        *tview.Form

  infoList           *tview.List

  exitModal          *tview.Modal
  errorModal         *tview.Modal

  inputChan           chan *NetData
  outputChan          chan int // NETDATA_* requests

  finish              chan error
}

func (cli *CLI) eventHandler(eventKey *tcell.EventKey) *tcell.EventKey {
  if eventKey.Rune() == 'q' {
    cli.pages.SwitchToPage("exit")
  }

  if eventKey.Rune() == 'b' {

  }

  return eventKey
}

func (cli *CLI) handleButton(btn string) {
  switch btn {
  case "call":
    cli.outputChan <- NETDATA_CALL
  case "check":
    cli.outputChan <- NETDATA_CHECK
  case "fold":
    cli.outputChan <- NETDATA_FOLD
  case "raise":
    cli.outputChan <- NETDATA_BET
  case "start":
    cli.outputChan <- NETDATA_STARTGAME
  case "quit":
    cli.pages.SwitchToPage("exit")
  }
}

func (cli *CLI) addNewPlayer(player *Player) {
  textView := tview.NewTextView()

  textView.SetTextAlign(tview.AlignCenter).
           SetBorder(true).
           SetTitle(player.Name)

  cli.otherPlayersFlexMap[player.Name] = textView

  cli.otherPlayersFlex.AddItem(textView, 0, 1, false)

  cli.app.Draw()
}

func (cli *CLI) removePlayer(player *Player) {
  textView := cli.otherPlayersFlexMap[player.Name]

  cli.otherPlayersFlex.RemoveItem(textView)
  
  cli.app.Draw()
}

func (cli *CLI) Init() error {
  cli.app        = tview.NewApplication()
  cli.pages      = tview.NewPages()
  cli.gameGrid   = tview.NewGrid()
  cli.exitModal  = tview.NewModal()
  cli.errorModal = tview.NewModal()

  cli.inputChan  = make(chan *NetData)
  cli.outputChan = make(chan int)
  cli.finish     = make(chan error)

  newTextView := func(title string, border bool, save **tview.TextView) *tview.TextView {
    ret := tview.NewTextView().SetTextAlign(tview.AlignCenter).
           SetChangedFunc(func() {
             cli.app.Draw()
           })

    if save != nil {
      *save = ret
    }

    ret.SetTitle(title)

    if border {
      ret.SetBorder(true)
    }
    
    return ret
  }

  //_tmp := "┌──────┐\n│ card |\n└──────┘"
  cli.otherPlayersFlex    = tview.NewFlex()
  cli.otherPlayersFlexMap = make(map[string]*tview.TextView)

  cli.actionsForm = tview.NewForm().
    AddInputField("bet amount", "", 20, nil, nil).
    AddButton("call",  func() {
      cli.handleButton("call")
    }).
    AddButton("raise", func() {
      cli.handleButton("raise")
    }).
    AddButton("fold",  func() {
      cli.handleButton("fold")
    }).
    AddButton("quit",  func() {
      cli.handleButton("quit")
    })
  cli.actionsForm.SetBorder(true).SetTitle("Actions")

  cli.infoList = tview.NewList().
    AddItem("# players",   "", '-', nil).
    AddItem("# connected", "", '-', nil).
    AddItem("buy in",      "", '-', nil).
    AddItem("big blind",   "", '-', nil).
    AddItem("small blind", "", '-', nil).
    AddItem("status",      "", '-', nil)
  cli.infoList.SetBorder(true).SetTitle("Table Info")

  cli.gameGrid.
    SetRows(0, 0, 0).
    SetColumns(0, 0, 0).
    AddItem(newTextView("Community Cards", true, &cli.commView),  0, 0, 1, 3, 0, 0, false).
    AddItem(cli.otherPlayersFlex,                                 1, 0, 1, 3, 0, 0, false).
    AddItem(cli.actionsForm,                                      2, 0, 1, 1, 0, 0, false).
    AddItem(newTextView("Hole",            true, &cli.holeView),  2, 1, 1, 1, 0, 0, false).
    AddItem(cli.infoList,                                         2, 2, 1, 1, 0, 0, false)

  cli.exitModal.SetText("do you want to quit the game?").
    AddButtons([]string{"quit", "cancel"}).
    SetDoneFunc(func(btnIdx int, btnLabel string) {
      switch btnLabel {
      case "quit":
        cli.app.Stop()
      case "cancel":
        cli.pages.SwitchToPage("game")
        cli.app.SetFocus(cli.actionsForm)
      }
    })

  cli.errorModal.
    AddButtons([]string{"close"}).
    SetDoneFunc(func(_ int, btnLabel string) {
      switch btnLabel {
      case "close":
        cli.pages.SwitchToPage("game")
        cli.app.SetFocus(cli.actionsForm)
        cli.errorModal.SetText("")
      }
    })

  cli.pages.AddPage("game",  cli.gameGrid,   true, true)
  cli.pages.AddPage("exit",  cli.exitModal,  true, false)
  cli.pages.AddPage("error", cli.errorModal, true, false)

  cli.app.SetInputCapture(cli.eventHandler)

  return nil
}

func (cli *CLI) InputChan() chan *NetData {
  return cli.inputChan
}

func (cli *CLI) OutputChan() chan int {
  return cli.outputChan
}

func (cli *CLI) Finish() chan error {
  return cli.finish
}

func cliInputLoop(cli *CLI) {
  for {
    select {
    case netData := <-cli.inputChan:
      switch netData.Response {
      case NETDATA_NEWCONN, NETDATA_CLIENTEXITED:
        cli.infoList.SetItemText(1, "# connected", strconv.FormatUint(uint64(netData.Table.NumConnected), 10))
      case NETDATA_NEWPLAYER, NETDATA_CURPLAYERS:
        assert(netData.PlayerData != nil, "PlayerData == nil")

        cli.addNewPlayer(netData.PlayerData)
      case NETDATA_PLAYERLEFT:
        cli.removePlayer(netData.PlayerData)
      case NETDATA_MAKEADMIN:
        cli.actionsForm.AddButton("start game", func() {
          cli.handleButton("start")
        })
      case NETDATA_STARTGAME:
        ;
      case NETDATA_DEAL:
        txt := "\n"
        if netData.PlayerData != nil {
          holeCards := netData.PlayerData.Hole.Cards

          // TODO: use a Box instead
          padCardOne, padCardTwo := " ", " "

          if holeCards[0].NumValue == 10 {
            padCardOne = ""
          }
          if holeCards[1].NumValue == 10 {
            padCardTwo = ""
          }

          txt += fmt.Sprintf("┌───────┐┌───────┐\n")
          txt += fmt.Sprintf("│ %s%s  ││ %s%s  │\n", holeCards[0].Name, padCardOne, holeCards[1].Name, padCardTwo)
          txt += fmt.Sprintf("└───────┘└───────┘\n")

          cli.holeView.SetText(txt)
        }
      case NETDATA_FLOP:
        txt := "\n"
        for _, card := range netData.Table.Community {
          txt += fmt.Sprintf("┌──────┐\n│ %s |\n└──────┘ ", card.Name)
        } ; txt += "\n"

        cli.commView.SetText(txt)
      case NETDATA_BADREQUEST:
        if (netData.Msg == "") {
          netData.Msg = "unspecified server error"
        }

        cli.errorModal.SetText(netData.Msg)
        cli.pages.SwitchToPage("error")
      default:
        panic("bad response")
      }

    case err := <-cli.finish:
      if err != nil {
        fmt.Printf("backend error: %s\n", err)
      }

      cli.app.Stop()

      return
    }
  }
}

func (cli *CLI) Run() error {
  go cliInputLoop(cli)

  if err := cli.app.SetRoot(cli.pages, true).SetFocus(cli.actionsForm).Run(); err != nil {
    return err
	}

  return nil
}
