package main

import (
  "fmt"
  "strconv"
  //"strings"

  "github.com/gdamore/tcell/v2"
  "github.com/rivo/tview"
)

type CLI struct {
  app               *tview.Application
  pages             *tview.Pages
  gameGrid          *tview.Grid

  curPage            string

  lastKey            rune

  yourName           string

  bet                uint
 
  commView,
  otherPlayersView,
  yourInfoView,
  holeView           *tview.TextView

  yourInfoFlex,
  otherPlayersFlex   *tview.Flex

  playersTextViewMap  map[string]*tview.TextView

  actionsFlex        *tview.Flex
  actionsForm        *tview.Form

  tableInfoList      *tview.List

  exitModal          *tview.Modal
  errorModal         *tview.Modal

  inputChan           chan *NetData
  outputChan          chan *NetData // will route CLI input to server

  finish              chan error
}

// TODO: check if page exists
func (cli *CLI) switchToPage(page string) {
  cli.curPage = page

  cli.pages.SwitchToPage(page)
}

func (cli *CLI) eventHandler(eventKey *tcell.EventKey) *tcell.EventKey {
  key := eventKey.Rune()

  if key == 'q' {
    cli.pages.SwitchToPage("exit")
    cli.app.SetFocus(cli.exitModal)

    return eventKey
  }

  if cli.curPage != "game" {
    return eventKey
  }

  switch key {
  case 'b':
    if cli.lastKey == 'b' {
      cli.lastKey = '.'
      cli.handleButton("bet")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetFormItem(0))
    }
  case 'c':
    if cli.lastKey == 'c' {
      cli.lastKey = '.'
      cli.handleButton("check")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetButton(0))
    }
  case 'r':
    if cli.lastKey == 'r' {
      cli.lastKey = '.'
      cli.handleButton("raise")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetButton(2))
    }
  case 'f':
    if cli.lastKey == 'f' {
      cli.lastKey = '.'
      cli.handleButton("fold")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetButton(3))
    }
  }

  cli.lastKey = key

  return eventKey
}

func (cli *CLI) handleButton(btn string) {
  req := NETDATA_BADREQUEST

  switch btn {
  case "call":
    req = NETDATA_CALL
  case "check":
    req = NETDATA_CHECK
  case "fold":
    req = NETDATA_FOLD
  case "raise":
    req = NETDATA_BET
  case "start":
    req = NETDATA_STARTGAME
  case "quit":
    cli.switchToPage("exit")
    cli.app.SetFocus(cli.exitModal)
    return
  }

  cli.outputChan <- &NetData{
    Request: req,
    PlayerData: &Player{ Action: Action{ Action: req, Amount: cli.bet, } }, // XXX: lol
  }
}

func (cli *CLI) updateInfoList(item string, table *Table) {
  switch item {
  case "# connected":
    cli.tableInfoList.SetItemText(2, "# connected",
      strconv.FormatUint(uint64(table.NumConnected), 10))
  case "# players":
    cli.tableInfoList.SetItemText(0, "# players",
      strconv.FormatUint(uint64(table.NumPlayers), 10))
    cli.tableInfoList.SetItemText(1, "# open seats",
      strconv.FormatUint(uint64(table.NumSeats) - uint64(table.NumPlayers), 10))
  case "status":
    cli.tableInfoList.SetItemText(3, "dealer",      table.Dealer.Name)
    cli.tableInfoList.SetItemText(4, "small blind", table.SmallBlind.Name)
    cli.tableInfoList.SetItemText(5, "big blind",   table.BigBlind.Name)
    cli.tableInfoList.SetItemText(6, "status",      table.TableStateToString())
  }

  cli.app.Draw()
}

func (cli *CLI) addNewPlayer(player *Player) {
  if player.Name == cli.yourName {
    return
  }

  textView := tview.NewTextView()

  textView.SetTextAlign(tview.AlignCenter).
           SetBorder(true).
           SetTitle(player.Name)

  cli.playersTextViewMap[player.Name] = textView

  cli.otherPlayersFlex.AddItem(textView, 0, 1, false)

  cli.app.Draw()
}

func (cli *CLI) removePlayer(player *Player) {
  if player.Name == cli.yourName {
    return
  }

  textView := cli.playersTextViewMap[player.Name]

  cli.otherPlayersFlex.RemoveItem(textView)
  
  cli.app.Draw()
}

func (cli *CLI) updatePlayer(player *Player) {
  textView := cli.playersTextViewMap[player.Name]

  if player.Name == cli.yourName {
    textView.
      SetText("name: "           + player.Name                                      + "\n" +
              "current action: " + player.ActionToString()                          + "\n" +
              "chip count: "     + strconv.FormatUint(uint64(player.ChipCount), 10) + "\n")
  } else {
    if (textView == nil) { // XXX
      return
    }
    /*tv := strings.Split(textView.GetText(false), "\n")
    textView.
      SetText(player.ActionToString()    + "\n" +
              strings.Join(tv[1:], "\n"))*/
    textView.
      SetText("current action: " + player.ActionToString() + "\n" +
              "chip count: "     + strconv.FormatUint(uint64(player.ChipCount), 10) + "\n\n")

    if (player.Hole != nil) {
      textView.
        SetText(textView.GetText(false) + cli.cards2String(player.Hole.Cards))
    }
  }
}

func (cli *CLI) Init() error {
  cli.app        = tview.NewApplication()
  cli.pages      = tview.NewPages()
  cli.gameGrid   = tview.NewGrid()
  cli.exitModal  = tview.NewModal()
  cli.errorModal = tview.NewModal()

  cli.curPage    = "game"

  cli.inputChan  = make(chan *NetData)
  cli.outputChan = make(chan *NetData)
  cli.finish     = make(chan error)

  newTextView := func(title string, border bool) *tview.TextView {
    ret := tview.NewTextView().SetTextAlign(tview.AlignCenter).
           SetChangedFunc(func() {
             cli.app.Draw()
           })

    ret.SetTitle(title)

    if border {
      ret.SetBorder(true)
    }
    
    return ret
  }

  cli.commView = newTextView("Community Cards", true)

  cli.otherPlayersFlex = tview.NewFlex()

  cli.playersTextViewMap = make(map[string]*tview.TextView)

  cli.actionsForm = tview.NewForm().
    AddInputField("bet amount", "", 20, func(inp string, lastChar rune) bool {
      if _, err := strconv.ParseUint(inp, 10, 64); err != nil {
        return false
      } else {
        return true
      }
    }, func(inp string) {
      n, err := strconv.ParseUint(inp, 10, 64)
      if err != nil {
        cli.bet = 0
      } else {
        cli.bet = uint(n) // XXX
      }
    }).
    AddButton("check", func() {
      cli.handleButton("check")
    }).
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
  cli.actionsForm.GetFormItem(0).SetFinishedFunc(func(_ tcell.Key) {
    label := cli.actionsForm.GetFormItem(0).GetLabel()

    _, err := strconv.ParseUint(label, 10, 64)
    if err != nil {
      // should never happen, input is checked in accept()
      panic("bad bet val: " + label)
    }
  })
  cli.actionsForm.SetBorder(true).SetTitle("Actions")

  cli.yourInfoFlex = tview.NewFlex().SetDirection(tview.FlexRow)

  cli.yourInfoView = newTextView("Your Info", true)
  cli.yourInfoView.SetTextAlign(tview.AlignLeft)
  cli.yourInfoView.SetText("name: \ncurrent action: \nchip count: \n")

  cli.holeView = newTextView("Hole", true)

  cli.yourInfoFlex.
    AddItem(cli.yourInfoView, 0, 1, false).
    AddItem(cli.holeView,     0, 1, false)

  cli.tableInfoList = tview.NewList().
    AddItem("# players",    "", '-', nil).
    AddItem("# open seats", "", '-', nil).
    AddItem("# connected",  "", '-', nil).
    AddItem("buy in",       "", '-', nil).
    AddItem("dealer",       "", '-', nil).
    AddItem("small blind",  "", '-', nil).
    AddItem("big blind",    "", '-', nil).
    AddItem("status",       "", '-', nil)
  cli.tableInfoList.SetBorder(true).SetTitle("Table Info")

  cli.gameGrid.
    SetRows(0, 0, 0).
    SetColumns(0, 0, 0).
    AddItem(cli.commView,         0, 0, 1, 3, 0, 0, false).
    AddItem(cli.otherPlayersFlex, 1, 0, 1, 3, 0, 0, false).
    AddItem(cli.actionsForm,      2, 0, 1, 1, 0, 0, false).
    AddItem(cli.yourInfoFlex,     2, 1, 1, 1, 0, 0, false).
    AddItem(cli.tableInfoList,    2, 2, 1, 1, 0, 0, false)

  cli.exitModal.SetText("do you want to quit the game?").
    AddButtons([]string{"quit", "cancel"}).
    SetDoneFunc(func(btnIdx int, btnLabel string) {
      switch btnLabel {
      case "quit":
        cli.app.Stop()
      case "cancel":
        cli.switchToPage("game")
        cli.app.SetFocus(cli.actionsForm)
      }
    })

  cli.errorModal.
    AddButtons([]string{"close"}).
    SetDoneFunc(func(_ int, btnLabel string) {
      switch btnLabel {
      case "close":
        cli.switchToPage("game")
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

func (cli *CLI) OutputChan() chan *NetData {
  return cli.outputChan
}

func (cli *CLI) Finish() chan error {
  return cli.finish
}

func (cli *CLI) cards2String(cards Cards) string {
  txt := "\n"

  for i := 0; i < len(cards); i++ {
    txt += fmt.Sprintf("┌───────┐")
  } ; txt += "\n"

  for _, card := range cards {
    pad := " "
    
    if card.NumValue == 10 {
      pad = ""
    }

    txt += fmt.Sprintf("│ %s%s  │", card.Name, pad)
  } ; txt += "\n"

  for i := 0; i < len(cards); i++ {
    txt += fmt.Sprintf("└───────┘")
  } ; txt += "\n"

  return txt
}

func cliInputLoop(cli *CLI) {
  for {
    select {
    case netData := <-cli.inputChan:
      switch netData.Response {
      case NETDATA_NEWCONN, NETDATA_CLIENTEXITED:
        assert(netData.Table != nil, "netData.Table == nil")

        cli.updateInfoList("# connected", netData.Table)
      case NETDATA_NEWPLAYER, NETDATA_CURPLAYERS:
        assert(netData.PlayerData != nil, "PlayerData == nil")
        
        cli.updateInfoList("# players", netData.Table)

        cli.addNewPlayer(netData.PlayerData)
      case NETDATA_PLAYERLEFT:
        cli.removePlayer(netData.PlayerData)
        cli.updateInfoList("# players", netData.Table)
      case NETDATA_MAKEADMIN:
        cli.actionsForm.AddButton("start game", func() {
          cli.handleButton("start")
        })
      case NETDATA_DEAL:
        if netData.PlayerData != nil {
          if cli.playersTextViewMap[netData.PlayerData.Name] == nil {
            cli.yourName = netData.PlayerData.Name
            cli.playersTextViewMap[netData.PlayerData.Name] = cli.yourInfoView
          }

          cli.updatePlayer(netData.PlayerData)

          txt := cli.cards2String(netData.PlayerData.Hole.Cards)

          cli.holeView.SetText(txt)
        }
      case NETDATA_PLAYERACTION:
        assert(netData.PlayerData != nil, "PlayerData == nil")

        cli.updatePlayer(netData.PlayerData)
        cli.updateInfoList("status", netData.Table)
      case NETDATA_SHOWHAND:
        assert(netData.PlayerData != nil, "Playerdata == nil")

        cli.updatePlayer(netData.PlayerData)
      case NETDATA_ROUNDOVER:
        //cli.updatePlayer(netData.PlayerData)
        cli.updateInfoList("status", netData.Table)
        cli.errorModal.SetText(netData.Msg)
        cli.switchToPage("error")
        
        for _, player := range netData.Table.Winners {
          cli.updatePlayer(player)
        }

        cli.app.Draw()
      case NETDATA_FLOP, NETDATA_TURN, NETDATA_RIVER:
        txt := cli.cards2String(netData.Table.Community)

        cli.commView.SetText(txt)
        cli.updateInfoList("status", netData.Table)
      case NETDATA_BADREQUEST:
        if (netData.Msg == "") {
          netData.Msg = "unspecified server error"
        }

        cli.errorModal.SetText(netData.Msg)
        cli.switchToPage("error")
        cli.app.Draw()
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
