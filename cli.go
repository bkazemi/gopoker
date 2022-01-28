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

type CLIFocusList struct {
  prev, next *CLIFocusList
  prim        tview.Primitive
}

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

  chatFlex           *tview.Flex
  chatTextView       *tview.TextView
  chatInputField     *tview.InputField
  chatMsg             string

  tableInfoList      *tview.List

  exitModal,
  errorModal         *tview.Modal

  focusList          *CLIFocusList

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

  switch eventKey.Key() {
  case tcell.KeyLeft:
    cli.focusList = cli.focusList.prev
    cli.app.SetFocus(cli.focusList.prim)
  case tcell.KeyRight:
    cli.focusList = cli.focusList.next
    cli.app.SetFocus(cli.focusList.prim)
  }

  // if we don't check this we will switch out of chat input when
  // the runes below are pressed
  if cli.app.GetFocus() == cli.chatInputField {
    return eventKey
  }

  switch key {
  case 'a':
    if cli.lastKey == 'a' {
      cli.lastKey = '_'
      cli.handleButton("allin")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetButton(3))
    }
  case 'b':
    if cli.lastKey == 'b' {
      cli.lastKey = '_'
      cli.handleButton("bet")
    } else {
      cli.app.SetFocus(cli.betInputField)
    }
  case 'c':
    if cli.lastKey == 'c' {
      cli.lastKey = '_'
      cli.handleButton("check")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetButton(0))
    }
  case 'r':
    if cli.lastKey == 'r' {
      cli.lastKey = '_'
      cli.handleButton("raise")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetButton(2))
    }
  case 'f':
    if cli.lastKey == 'f' {
      cli.lastKey = '_'
      cli.handleButton("fold")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetButton(4))
    }

  case 'm':
    cli.app.SetFocus(cli.chatInputField)
    cli.chatInputField.SetText("") // NOTE: otherwise `m` shows up in field

  case 's':
    if cli.actionsForm.GetButtonCount() < 7 {
      break
    }

    if cli.lastKey == 's' {
      cli.lastKey = '_'
      cli.handleButton("start")
    } else {
      cli.app.SetFocus(cli.actionsForm.GetButton(6))
    }
  }

  cli.lastKey = key

  return eventKey
}

func (cli *CLI) handleButton(btn string) {
  req := NETDATA_BADREQUEST
  msg := ""

  switch btn {
  case "allin":
    req = NETDATA_ALLIN
  case "call":
    req = NETDATA_CALL
  case "check":
    req = NETDATA_CHECK
  case "fold":
    req = NETDATA_FOLD
  case "raise":
    req = NETDATA_BET
    cli.betInputField.SetText("")
  case "msg":
    req = NETDATA_CHATMSG
    msg = cli.chatMsg

    cli.chatMsg = ""
  case "start":
    req = NETDATA_STARTGAME
  case "quit":
    cli.switchToPage("exit")
    cli.app.SetFocus(cli.exitModal)
    return
  }

  cli.outputChan <- &NetData{
    Request: req,
    PlayerData: &Player{
      Action: Action{ Action: req, Amount: cli.bet, },
    }, // XXX action x3!
    Msg: msg,
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
    cli.tableInfoList.SetItemText(4, "pot",         table.PotToString())
    cli.tableInfoList.SetItemText(5, "dealer",      table.Dealer.Name)
    cli.tableInfoList.SetItemText(6, "small blind", table.SmallBlindToString())
    cli.tableInfoList.SetItemText(7, "big blind",   table.BigBlindToString())
    cli.tableInfoList.SetItemText(8, "status",      table.TableStateToString())
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
      assembleBestHand(true, table, player)

      preHand := player.PreHand.RankName()
      textViewSetLine(textView, 4, "current hand: " + preHand)

      //return XXX update hand seperately
    }

    textViewSetLine(textView, 1, "name: "           + player.Name)
    textViewSetLine(textView, 2, "current action: " + player.ActionToString())
    textViewSetLine(textView, 3, "chip count: "     + player.ChipCountToString())

    textView.ScrollToBeginning() // XXX find out why extra empty lines are being added to yourInfoView
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

func (cli *CLI) updateChat(msg string) {
  cli.chatTextView.SetText(cli.chatTextView.GetText(false) + msg)
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

  cli.chatTextView = newTextView("chat", false)
  cli.chatTextView.SetTextAlign(tview.AlignLeft)

  cli.chatInputField = tview.NewInputField()
  cli.chatInputField.
    SetLabel("msg: ").
    SetFinishedFunc(func(_ tcell.Key) {
      msg := cli.chatInputField.GetText()

      if strings.TrimSpace(msg) == "" {
        return
      }

      cli.chatMsg = msg
      cli.chatInputField.SetText("")

      cli.handleButton("msg")
    })

  cli.chatFlex = tview.NewFlex().SetDirection(tview.FlexRow).
    AddItem(cli.chatTextView,   0, 8, false).
    AddItem(tview.NewBox(),     0, 1, false).
    AddItem(cli.chatInputField, 0, 1, false)
  cli.chatFlex.SetBorder(true).SetTitle("Chat")

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
      if inp == "" {
        return
      }

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
    AddButton("all-in", func() {
      cli.handleButton("allin")
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
    AddItem(tview.NewBox(),     0, 1, false).
    AddItem(cli.betInputField,  0, 1, false).
    AddItem(cli.actionsForm,    0, 8, false)
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
    AddItem("pot",          "", '-', nil).
    AddItem("dealer",       "", '-', nil).
    AddItem("small blind",  "", '-', nil).
    AddItem("big blind",    "", '-', nil).
    AddItem("status",       "", '-', nil)
  cli.tableInfoList.SetBorder(true).SetTitle("Table Info")

  cli.gameGrid.
    SetRows(0, 0, 0).
    SetColumns(0, 0, 0).
    AddItem(cli.commView,         0, 0, 1, 2, 0, 0, false).
    AddItem(cli.chatFlex,         0, 2, 1, 1, 0, 0, false).
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

  cli.focusList = &CLIFocusList{
    prev: &CLIFocusList{ prim: cli.tableInfoList },
    prim: cli.actionsForm,
  }
  cli.focusList.next = &CLIFocusList{
    prev: cli.focusList,
    next: cli.focusList.prev,
    prim: cli.yourInfoView,
  }
  cli.focusList.prev.prev = cli.focusList.next
  cli.focusList.prev.next = cli.focusList

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
      case NETDATA_CHATMSG:
        cli.updateChat(netData.Msg)
      case NETDATA_YOURPLAYER:
        assert(netData.PlayerData != nil, "PlayerData == nil")
        cli.updateInfoList("# players", netData.Table)

        cli.yourName = netData.PlayerData.Name
        cli.playersTextViewMap[cli.yourName] = cli.yourInfoView

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

        // set focus in case the user was focused on chat
        if curPlayerTextView == cli.yourInfoView {
          if page, _ := cli.pages.GetFrontPage(); page == "game" {
            cli.app.SetFocus(cli.actionsForm)
          }
        }

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
          cli.playersTextViewMap[cli.yourName].SetTextAlign(tview.AlignCenter)
          cli.playersTextViewMap[cli.yourName].SetText("eliminated")
          cli.holeView.Clear()
          cli.switchToPage("error")

          cli.app.Draw()
        }

        //cli.removePlayer(netData.PlayerData)
      case NETDATA_FLOP, NETDATA_TURN, NETDATA_RIVER:
        txt := cli.cards2String(netData.Table.Community)

        cli.commView.SetText(txt)
        cli.updateInfoList("status", netData.Table)

      case NETDATA_BADREQUEST, NETDATA_SERVERMSG:
        if netData.Msg == "" {
          if netData.Response == NETDATA_BADREQUEST {
            netData.Msg = "unspecified server error"
          } else {
            netData.Msg = "BUG: empty server message"
          }
        }

        cli.errorModal.SetText(netData.Msg)
        cli.switchToPage("error")
        cli.app.Draw()
      case NETDATA_SERVERCLOSED:
        cli.finish <- errors.New("server closed")
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
