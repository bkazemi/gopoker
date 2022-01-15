package main

import (
  "fmt"
  "strconv"
  "errors"
  "strings"
  //"os"

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
  curPlayerTextView  *tview.TextView

  betInputField      *tview.InputField
  actionsFlex        *tview.Flex
  actionsBox         *tview.Box
  actionsForm        *tview.Form

  tableInfoList      *tview.List

  exitModal          *tview.Modal
  errorModal         *tview.Modal

  inputChan           chan *NetData
  outputChan          chan *NetData // will route CLI input to server

  finish              chan error
  err                 chan error
  done                chan bool // XXX for waiting on quit button during premature exits
}

// TODO: check if page exists
func (cli *CLI) switchToPage(page string) {
  if page == "errorMustQuit" { // irrecoverable error
    cli.errorModal.ClearButtons()

    cli.errorModal.
      AddButtons([]string{"quit"}).
      SetDoneFunc(func(_ int, btnLabel string) {
        switch btnLabel {
        case "quit":
          cli.done <- true
        }
      })

    cli.pages.SwitchToPage("error")
    cli.app.SetFocus(cli.errorModal)
  } else {
    cli.curPage = page

    cli.pages.SwitchToPage(page)
  }
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
      cli.app.SetFocus(cli.betInputField)
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

  case 's':
    if cli.lastKey == 's' {
      cli.lastKey = '.'
      cli.handleButton("start")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetButton(5))
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
      strconv.FormatUint(uint64(table.GetNumOpenSeats()), 10))
  case "status":
    cli.tableInfoList.SetItemText(4, "dealer",      table.Dealer.Name)
    cli.tableInfoList.SetItemText(5, "small blind", table.SmallBlindToString())
    cli.tableInfoList.SetItemText(6, "big blind",   table.BigBlindToString())
    cli.tableInfoList.SetItemText(7, "status",      table.TableStateToString())
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

func (cli *CLI) updatePlayer(player *Player, table *Table) {
  textView := cli.playersTextViewMap[player.Name]

  if player.Name == cli.yourName {
    if table != nil {
      assemble_best_hand(true, table, player)

      preHand := player.PreHand.RankName()
      textViewSetLine(textView, 4, "current hand: " + preHand)

      return
    }

    textViewSetLine(textView, 1, "name: "           + player.Name)
    textViewSetLine(textView, 2, "current action: " + player.ActionToString())
    textViewSetLine(textView, 3, "chip count: "     + player.ChipCountToString())
  } else {
    if (textView == nil) { // XXX
      return
    }

    textViewSetLine(textView, 1, "current action: " + player.ActionToString())
    textViewSetLine(textView, 2, "chip count: "     + player.ChipCountToString() + "\n")

    if (player.Hole != nil) {
      textViewSetLine(textView, 3, cli.cards2String(player.Hole.Cards))
    }
  }

}

func (cli *CLI) clearPlayerScreens() {
  for _, textView := range cli.playersTextViewMap {
    textView.Clear()
  }
}

// NOTE: make sure to call this with increasing lineNums
//       on first draws
func textViewSetLine(textView *tview.TextView, lineNum int, txt string) {
  if lineNum < 1 {
    return
  }

  tv := strings.Split(textView.GetText(false), "\n")

  if lineNum > len(tv) { // line not added yet
    textView.SetText(textView.GetText(false) + txt)
  } else if tv[lineNum-1] == txt {
    return // no update
  } else {
    tv[lineNum-1] = txt
    textView.SetText(strings.Join(tv, "\n"))
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
  cli.finish     = make(chan error, 1)
  cli.err        = make(chan error, 1)

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

  cli.betInputField = tview.NewInputField() // XXX compiler claims I need a type assertion if I chain here?
  cli.betInputField.
    SetLabel("bet amount: ").
    SetFieldWidth(20).
    SetAcceptanceFunc(func(inp string, lastChar rune) bool {
      if _, err := strconv.ParseUint(inp, 10, 64); err != nil {
        return false
      } else {
        return true
      }
    }).
    SetChangedFunc(func(inp string) {
      n, err := strconv.ParseUint(inp, 10, 64)
      if err != nil {
        cli.bet = 0
      } else {
        cli.bet = uint(n) // XXX
      }
    }).
    SetFinishedFunc(func(_ tcell.Key) {
      n := cli.betInputField.GetText()

      if n == "" {
        return
      }

      _, err := strconv.ParseUint(n, 10, 64)
      if err != nil {
        // should never happen, input is checked in accept func
        panic("bad bet val: " + n)
      }
    })

  cli.actionsForm = tview.NewForm().SetHorizontal(true).
    // XXX ask dev about button overflow
    /*AddInputField("bet amount", "", 20, func(inp string, lastChar rune) bool {
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
    }).*/
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
  /*cli.actionsForm.GetFormItem(0).SetFinishedFunc(func(_ tcell.Key) {
    label := cli.actionsForm.GetFormItem(0).GetLabel()

    _, err := strconv.ParseUint(label, 10, 64)
    if err != nil {
      // should never happen, input is checked in accept()
      panic("bad bet val: " + label)
    }
  })*/
  cli.actionsForm.SetBorder(false)//.SetTitle("Actions")

  cli.actionsFlex = tview.NewFlex().SetDirection(tview.FlexRow).
    AddItem(tview.NewBox(),    0, 1, false).
    AddItem(cli.betInputField, 0, 1, false).
    AddItem(cli.actionsForm,   0, 8, false)
  cli.actionsFlex.SetBorder(true).SetTitle("Actions")

  cli.yourInfoFlex = tview.NewFlex().SetDirection(tview.FlexRow)

  cli.yourInfoView = newTextView("Your Info", true)
  cli.yourInfoView.SetTextAlign(tview.AlignLeft)
  //cli.yourInfoView.SetText("name: \ncurrent action: \nchip count: \n")

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
    AddItem(cli.actionsFlex,      2, 0, 1, 1, 0, 0, false).
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

func (cli *CLI) Error() chan error {
  return cli.err
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
  defer cli.app.Stop()

  for {
    select {
    case netData := <-cli.inputChan:
      switch netData.Response {
      case NETDATA_NEWCONN, NETDATA_CLIENTEXITED:
        assert(netData.Table != nil, "netData.Table == nil")

        cli.updateInfoList("# connected", netData.Table)
      case NETDATA_YOURPLAYER:
        assert(netData.PlayerData != nil, "PlayerData == nil")
        cli.updateInfoList("# players", netData.Table)

        cli.updatePlayer(netData.PlayerData, nil)
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
        cli.commView.Clear()
        cli.clearPlayerScreens()

        if netData.PlayerData != nil {
          if cli.playersTextViewMap[netData.PlayerData.Name] == nil {
            cli.yourName = netData.PlayerData.Name
            cli.playersTextViewMap[netData.PlayerData.Name] = cli.yourInfoView
          }

          cli.updatePlayer(netData.PlayerData, netData.Table)

          txt := cli.cards2String(netData.PlayerData.Hole.Cards)

          cli.holeView.SetText(txt)
        }
      case NETDATA_PLAYERACTION:
        assert(netData.PlayerData != nil, "PlayerData == nil")

        cli.updatePlayer(netData.PlayerData, netData.Table)
        cli.updateInfoList("status", netData.Table)
      case NETDATA_PLAYERTURN:
        assert(netData.PlayerData != nil, "PlayerData == nil")

        curPlayerTextView := cli.playersTextViewMap[netData.PlayerData.Name]

        if curPlayerTextView == nil {
          continue // XXX probably would be a bug
        }

        if cli.curPlayerTextView != nil {
          cli.curPlayerTextView.SetBorderColor(tcell.ColorWhite)
        }

        cli.curPlayerTextView = curPlayerTextView

        cli.curPlayerTextView.SetBorderColor(tcell.ColorRed)

        cli.app.Draw()
      case NETDATA_UPDATEPLAYER:
        assert(netData.PlayerData != nil, "PlayerData == nil")

        cli.updatePlayer(netData.PlayerData, netData.Table)
      case NETDATA_UPDATETABLE:
        assert(netData.Table != nil, "Table == nil")

        cli.updateInfoList("status", netData.Table)
      case NETDATA_CURHAND:
        assert(netData.PlayerData != nil, "PlayerData == nil")

        cli.updatePlayer(netData.PlayerData, netData.Table)
      case NETDATA_SHOWHAND:
        assert(netData.PlayerData != nil, "Playerdata == nil")

        cli.updatePlayer(netData.PlayerData, nil)
      case NETDATA_ROUNDOVER:
        //cli.updatePlayer(netData.PlayerData)
        cli.updateInfoList("status", netData.Table)
        cli.errorModal.SetText(netData.Msg)
        cli.switchToPage("error")
        
        for _, player := range netData.Table.Winners {
          cli.updatePlayer(player, nil)
        }

        cli.app.Draw()
      case NETDATA_ELIMINATED:
        if netData.PlayerData.Name == cli.yourName {
          cli.errorModal.SetText("you were eliminated")
          cli.switchToPage("error")

          cli.app.Draw()
        }

        //cli.removePlayer(netData.PlayerData)
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
        cli.finish <- errors.New("bad response")
      }

    case err := <-cli.finish: // XXX
      if err != nil {
        cli.done = make(chan bool, 1)

        cli.errorModal.SetText(fmt.Sprintf("backend error: %s", err))
        cli.switchToPage("errorMustQuit")
        cli.app.Draw()

        <-cli.done // wait for user to press close
        return
      } else {
        return
      }
    }
  }
}

func (cli *CLI) Run() error {
  go cliInputLoop(cli)

  defer func() {
    if err := recover(); err != nil {
      if cli.app != nil {
        cli.err <- err.(error)
        cli.app.Stop()
      }
    }
  }()

  if err := cli.app.SetRoot(cli.pages, true).SetFocus(cli.actionsForm).Run(); err != nil {
    return err
	}

  return nil
}
