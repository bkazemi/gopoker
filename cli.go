package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
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

  pagesToPrimFocus   map[string]tview.Primitive

  lastKey            rune

  isTableAdmin       bool
  yourClient         Client

  bet                 Chips
  betAddedCommas      bool
  betMultiplier       uint64
  betInputReplacer   *strings.Replacer

  commView,
  otherPlayersView,
  yourInfoView,
  holeView           *tview.TextView

  yourInfoFlex,
  otherPlayersFlex   *tview.Flex

  playersTextViewMap  map[string]*tview.TextView
  curPlayerTextView  *tview.TextView
  playerHeadTextView *tview.TextView

  settingsForm       *tview.Form
  settingsFlex       *tview.Flex
  settings           ClientSettings

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
  } else {
    cli.pages.SwitchToPage(page)
  }

  cli.app.SetFocus(cli.pagesToPrimFocus[page])
}

func (cli *CLI) eventHandler(eventKey *tcell.EventKey) *tcell.EventKey {
  // first of two universal keys is esc to quit the game
  if eventKey.Key() == tcell.KeyEscape {
    cli.switchToPage("exit")
  }

  // the second is Ctrl-R to refresh the screen in case something writes
  // to std{out,err} (e.g. when you are using gdb)
  if eventKey.Key() == tcell.KeyCtrlR {
    focusedPrim := cli.app.GetFocus()
    cli.app.SetRoot(cli.pages, true)
    cli.app.SetFocus(focusedPrim)
  }

  return eventKey
}

func (cli *CLI) handleButton(btn string) {
  msg := ""

  switch btn {
  case "raise":
    cli.betInputField.SetText("")
  case "msg":
    msg = cli.chatMsg

    cli.chatMsg = ""
  case "quit":
    cli.switchToPage("exit")
    return
  }

  buttonLabelRequestMap := map[string]NetAction{
    "all-in":     NetDataAllIn,
    "call":       NetDataCall,
    "check":      NetDataCheck,
    "fold":       NetDataFold,
    "raise":      NetDataBet,
    "msg":        NetDataChatMsg,
    "start game": NetDataStartGame,
    "settings":   NetDataClientSettings,
  }

  netData := &NetData{
    Client: &cli.yourClient,
  }

  if req, ok := buttonLabelRequestMap[btn]; ok {
    if req & NetActionNeedsActionBitMask != 0 && cli.yourClient.Player != nil {
      cli.yourClient.Player.Action.Action = req
      cli.yourClient.Player.Action.Amount = cli.bet
    }

    netData.Request = req
    netData.Msg = msg

    if req == NetDataClientSettings { // requested settings update
      netData.Client.Settings = &cli.settings
    }

    cli.outputChan <- netData
  } else {
    netData.Request = NetDataBadRequest
    cli.outputChan <- netData
  }
}

func (cli *CLI) updateInfoList(item string, table *Table) {
  switch item {
  case "all":
    fallthrough
  case "# connected":
    cli.tableInfoList.SetItemText(2, "# connected",
      strconv.FormatUint(table.NumConnected, 10))
  case "# players":
    cli.tableInfoList.SetItemText(0, "# players",
      strconv.FormatUint(uint64(table.NumPlayers), 10))
    cli.tableInfoList.SetItemText(1, "# open seats",
      strconv.FormatUint(uint64(table.GetNumOpenSeats()), 10))
  case "status":
    cli.tableInfoList.SetItemText(4, "pot",         table.PotToString())
    cli.tableInfoList.SetItemText(5, "dealer",      table.DealerToString())
    cli.tableInfoList.SetItemText(6, "small blind", table.SmallBlindToString())
    cli.tableInfoList.SetItemText(7, "big blind",   table.BigBlindToString())
    cli.tableInfoList.SetItemText(8, "status",      table.TableStateToString())
  }
}

func (cli *CLI) addNewPlayer(client *Client) {
  if client.ID == cli.yourClient.ID {
    return
  }

  textView := tview.NewTextView()

  textView.SetTextAlign(tview.AlignCenter).
           SetBorder(true).
           SetTitle(client.Player.Name)

  cli.playersTextViewMap[client.ID] = textView

  cli.otherPlayersFlex.AddItem(textView, 0, 1, false)
}

func (cli *CLI) removePlayer(client *Client) {
  if client.ID == cli.yourClient.ID {
    return
  }

  textView := cli.playersTextViewMap[client.ID]

  cli.otherPlayersFlex.RemoveItem(textView)
}

//var postOut string

func (cli *CLI) updatePlayer(client *Client, table *Table) {
  textView := cli.playersTextViewMap[client.ID]
  //postOut += fmt.Sprintf("updatePlayer(): %s ID is %s tvTitle %s\n", player.Name, clientID, textView.GetTitle())

  if client.ID == cli.yourClient.ID {
    if table != nil {
      assembleBestHand(true, table, client.Player)

      preHand := client.Player.PreHand.RankName()
      textViewSetLine(textView, 4, "current hand: " + preHand)

      //return XXX update hand seperately
    }

    textViewSetLine(textView, 1, "name: "           + client.Name)
    textViewSetLine(textView, 2, "current action: " + client.Player.ActionToString())
    textViewSetLine(textView, 3, "chip count: "     + client.Player.ChipCountToString())

    textView.ScrollToBeginning() // XXX find out why extra empty lines are being added to yourInfoView
  } else {
    if (textView == nil) { // XXX
      return
    }

    textView.SetTitle(client.Name)
    textViewSetLine(textView, 1, "current action: " + client.Player.ActionToString())
    textViewSetLine(textView, 2, "chip count: "     + client.Player.ChipCountToString())
    if client.Player.Hole != nil {
      textViewSetLine(textView, 3, "hand: " + client.Player.Hand.RankName() + "\n")
      textViewSetLine(textView, 4, cli.cards2String(client.Player.Hole.Cards))
    }
  }
}

func (cli *CLI) clearPlayerScreens() {
  for _, textView := range cli.playersTextViewMap {
    textView.Clear()
  }
}

func (cli *CLI) unmakeAdmin() {
  if cli.isTableAdmin {
    if startGameBtnIdx := cli.actionsForm.GetButtonIndex("start game");
       startGameBtnIdx != -1 {
      cli.actionsForm.RemoveButton(startGameBtnIdx)
    }

    if adminHeaderIdx := cli.settingsForm.GetFormItemIndex("admin options");
       adminHeaderIdx != -1 {
      cli.settingsForm.RemoveFormItem(adminHeaderIdx)
    }

    if tableLockDropDownIdx := cli.settingsForm.GetFormItemIndex("table lock");
       tableLockDropDownIdx != -1 {
      cli.settingsForm.RemoveFormItem(tableLockDropDownIdx)
    }

    if passwordIdx := cli.settingsForm.GetFormItemIndex("table password");
       passwordIdx != -1 {
      cli.settingsForm.RemoveFormItem(passwordIdx)
    }

    cli.isTableAdmin = false // XXX
  }
}

func (cli *CLI) updateChat(client *Client, msg string) {
  if msg == "" {
    return
  }

  cli.chatTextView.SetText(cli.chatTextView.GetText(false) + msg)

  if client == nil || client.ID != cli.yourClient.ID {
    cli.chatFlex.SetBorderColor(tcell.ColorGreen)
  }
}

// NOTE: make sure to call this with increasing lineNums
//       on first draws
func textViewSetLine(textView *tview.TextView, lineNum int, txt string) {
  if lineNum < 1 {
    return
  }

  tvStr := strings.ReplaceAll(textView.GetText(false), "\n\n", "\n")
  tv := strings.Split(tvStr, "\n")

  lineCnt := strings.Count(txt, "\n")

  if lineNum > len(tv) { // line not added yet
    tv = append(tv, txt)
    textView.SetText(strings.Join(tv, "\n") + "\n")
  } else if lineCnt == 1 && tv[lineNum-1] == txt {
    return // no update
  } else {
    if lineCnt > 1 {
      for i, _ := range tv[lineNum:minUInt64(uint64(lineNum+lineCnt-1),
                           uint64(len(tv)))] {
        tv[i] = ""
      }
    }
    tv[lineNum-1] = txt
    textView.SetText(strings.Join(tv, "\n") + "\n")
  }
}

func (cli *CLI) Init() error {
  cli.app        = tview.NewApplication()
  cli.pages      = tview.NewPages()
  cli.gameGrid   = tview.NewGrid()
  cli.exitModal  = tview.NewModal()
  cli.errorModal = tview.NewModal()

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
      cli.chatFlex.SetBorderColor(tcell.ColorWhite)

      cli.handleButton("msg")
    })
  cli.chatInputField.
    SetBlurFunc(func() {
      cli.chatFlex.SetBorderColor(tcell.ColorWhite)
    })

  cli.chatFlex = tview.NewFlex().SetDirection(tview.FlexRow).
    AddItem(cli.chatTextView,   0, 8, false).
    AddItem(tview.NewBox(),     0, 1, false).
    AddItem(cli.chatInputField, 0, 1, false)
  cli.chatFlex.SetBorder(true).SetTitle("Chat").
    SetFocusFunc(func() {
      cli.chatFlex.SetBorderColor(tcell.ColorWhite)
      cli.app.SetFocus(cli.chatInputField)
    }).
    SetBlurFunc(func() {
      cli.chatFlex.SetBorderColor(tcell.ColorWhite)
    })

  cli.otherPlayersFlex = tview.NewFlex()

  cli.playersTextViewMap = make(map[string]*tview.TextView)

  cli.betInputReplacer = strings.NewReplacer(",", "", "k", "", "m", "")
  cli.betMultiplier = 1

  cli.betInputField = tview.NewInputField() // XXX compiler claims I need a type assertion if I chain here?
  cli.betInputField.
    SetLabel("bet amount: ").
    SetFieldWidth(20).
    SetAcceptanceFunc(func(inp string, lastChar rune) bool {
      if lastChar == 'k' {
        cli.betMultiplier = 1000
      } else if lastChar == 'm' {
        cli.betMultiplier = 1e6
      }
      inp = cli.betInputReplacer.Replace(inp)
      if _, err := strconv.ParseUint(inp, 10, 64); err != nil {
        return false
      }

      cli.betAddedCommas = false
      return true
    }).
    SetChangedFunc(func(inp string) {
      if inp == "" {
        return
      }

      inp = cli.betInputReplacer.Replace(inp)
      n, err := strconv.ParseUint(inp, 10, 64)
      if err != nil {
        cli.bet = 0
      } else {
        cli.bet = Chips(n * cli.betMultiplier)
      }

      if !cli.betAddedCommas || cli.betMultiplier != 1 {
        go func() { // XXX yep..
            cli.app.QueueUpdateDraw(func() {
              cli.betInputField.SetText(printer.Sprintf("%d", cli.bet))
            })
        }()
        cli.betAddedCommas = true
        cli.betMultiplier = 1
      }
    }).
    SetFinishedFunc(func(_ tcell.Key) {
      n := cli.betInputField.GetText()

      if n == "" {
        return
      }

      cli.actionsForm.SetFocus(cli.actionsForm.GetButtonIndex("raise"))
      cli.app.SetFocus(cli.actionsForm)
    })
    cli.betInputField.
    SetInputCapture(func(eventKey *tcell.EventKey) *tcell.EventKey {
      switch eventKey.Key() {
      case tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete:
        go func() {
          cli.app.QueueUpdateDraw(func() {
            if len(cli.betInputField.GetText()) == 0 {
              cli.betInputField.SetText("")
              cli.bet = 0
            } else {
              cli.betInputField.SetText(printer.Sprintf("%d", cli.bet))
              cli.betAddedCommas = true
            }
          })
        }()
      }

      return eventKey
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
    AddButton("call", func() {
      cli.handleButton("call")
    }).
    AddButton("raise", func() {
      cli.handleButton("raise")
    }).
    AddButton("all-in", func() {
      cli.handleButton("all-in")
    }).
    AddButton("fold", func() {
      cli.handleButton("fold")
    }).
    AddButton("quit", func() {
      cli.handleButton("quit")
    }).
    AddButton("settings", func() {
      cli.switchToPage("settings")
    })
  cli.actionsForm.SetBorder(false).
    SetInputCapture(func(eventKey *tcell.EventKey) *tcell.EventKey {
     switch eventKey.Key() {
     case tcell.KeyLeft:
       _, idx := cli.actionsForm.GetFocusedItemIndex()

       if idx == 0 {
         idx = cli.actionsForm.GetFormItemCount() + cli.actionsForm.GetButtonCount() - 1
       } else {
         idx--
       }

       cli.actionsForm.SetFocus(idx)
       cli.app.SetFocus(cli.actionsForm)
     case tcell.KeyRight:
       _, idx := cli.actionsForm.GetFocusedItemIndex()

       if idx == cli.actionsForm.GetFormItemCount() + cli.actionsForm.GetButtonCount() {
         idx = 0
       } else {
         idx++
       }

       cli.actionsForm.SetFocus(idx)
       cli.app.SetFocus(cli.actionsForm)
     }

      return eventKey
    })

  cli.settingsForm = tview.NewForm().
    AddInputField("name", cli.yourClient.Name, 15, nil, func(newName string) {
        cli.yourClient.Name = newName
        cli.settings.Name = newName
    }).
    AddButton("request changes", func() {
      cli.handleButton("settings")
      cli.switchToPage("game")
    }).
    AddButton("cancel", func() {
      cli.switchToPage("game")
  })
  cli.settingsForm.SetFocus(0).SetBorder(true).SetTitle("Settings").
    SetBlurFunc(func() {
      cli.settingsForm.SetFocus(0)
  })

  cli.settingsFlex = tview.NewFlex().
    AddItem(nil, 0, 1, false).
    AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
              AddItem(nil, 0, 1, false).
              AddItem(cli.settingsForm, 0, 1, true).
              AddItem(nil, 0, 1, false), 0, 1, true).
    AddItem(nil, 0, 1, false)

  cli.actionsFlex = tview.NewFlex().SetDirection(tview.FlexRow).
    AddItem(tview.NewBox(),     0, 1, false).
    AddItem(cli.betInputField,  0, 1, false).
    AddItem(cli.actionsForm,    0, 8, false)
  cli.actionsFlex.SetBorder(true).SetTitle("Actions").
    SetFocusFunc(func(){
      cli.app.SetFocus(cli.actionsForm)
    }).
    SetInputCapture(func(eventKey *tcell.EventKey) *tcell.EventKey {
      switch eventKey.Key() {
      case tcell.KeyUp, tcell.KeyDown:
        if cli.betInputField.HasFocus() {
          cli.app.SetFocus(cli.actionsForm)
        } else {
          cli.app.SetFocus(cli.betInputField)
        }
      }

      return eventKey
  })

  cli.yourInfoFlex = tview.NewFlex().SetDirection(tview.FlexRow)

  cli.yourInfoView = newTextView("Your Info", true)
  cli.yourInfoView.SetTextAlign(tview.AlignLeft)

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
    AddItem(cli.tableInfoList,    2, 2, 1, 1, 0, 0, false).
    SetFocusFunc(func(){
      cli.app.SetFocus(cli.focusList.prim)
    }).
    SetInputCapture(func(eventKey *tcell.EventKey) *tcell.EventKey {
      defer func() {
        cli.lastKey = eventKey.Rune()
      }()

      switch eventKey.Key() {
      case tcell.KeyTab:
        if !cli.focusList.prim.HasFocus() {
          cli.app.SetFocus(cli.focusList.prim)
        } else {
          cli.focusList = cli.focusList.next
          cli.app.SetFocus(cli.focusList.prim)
        }

        return eventKey
      }

      if cli.chatFlex.HasFocus() {
        return eventKey
      }

      keyToActionsButtonLabel := map[rune]string{
          'a': "all-in",
          'c': "check",
          'C': "call",
          'r': "raise",
          'f': "fold",
          's': "start game",
          '.': "settings",
      }

      keyRune := eventKey.Rune()
      if label, ok := keyToActionsButtonLabel[keyRune]; ok {
        if cli.lastKey == keyRune {
          if label == "settings" {
            cli.switchToPage("settings")
          } else if label == "start game" && !cli.isTableAdmin {
            return nil
          } else {
            cli.handleButton(label)
          }
        } else {
          buttonIdx := cli.actionsForm.GetButtonIndex(label)
          if buttonIdx == -1 {
            return nil
          }

          cli.actionsForm.SetFocus(buttonIdx)
          cli.app.SetFocus(cli.actionsFlex)
        }

        return nil
      }

      switch keyRune {
      case 'b':
        cli.app.SetFocus(cli.betInputField)
      case 'q':
        cli.switchToPage("exit")
      case 'm':
        if cli.app.GetFocus() != cli.betInputField {
          cli.app.SetFocus(cli.chatFlex)
          return nil
        }
      }

      return eventKey
  })

  cli.exitModal.SetText("do you want to quit the game?").
    AddButtons([]string{"quit", "cancel"}).
    SetDoneFunc(func(btnIdx int, btnLabel string) {
      switch btnLabel {
      case "quit":
        cli.app.Stop()
      case "cancel":
        cli.switchToPage("game")
      }
  })

  cli.errorModal.
    AddButtons([]string{"close"}).
    SetDoneFunc(func(_ int, btnLabel string) {
      switch btnLabel {
      case "close":
        cli.switchToPage("game")
        cli.errorModal.SetText("")
      }
  })

  cli.focusList = &CLIFocusList{
    prev: &CLIFocusList{ prim: cli.tableInfoList },
    prim: cli.actionsFlex,
  }
  cli.focusList.next = &CLIFocusList{
    prev: cli.focusList,
    next: cli.focusList.prev,
    prim: cli.yourInfoView,
  }
  cli.focusList.prev.prev = cli.focusList.next
  cli.focusList.prev.next = cli.focusList

  cli.pages.AddPage("game",     cli.gameGrid,     true, true)
  cli.pages.AddPage("exit",     cli.exitModal,    true, false)
  cli.pages.AddPage("error",    cli.errorModal,   true, false)
  cli.pages.AddPage("settings", cli.settingsFlex, true, false)

  // XXX: i probably shouldn't need this. sometimes pages weren't being focused
  //       properly back when i was learning the library. check back
  cli.pagesToPrimFocus = map[string]tview.Primitive{
    "game":          cli.gameGrid,
    "exit":          cli.exitModal,
    "error":         cli.errorModal,
    "errorMustQuit": cli.errorModal,
    "settings":      cli.settingsFlex,
  }

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
  if len(cards) == 0 {
    return ""
  }

  txt := "\n"
  for i := 0; i < len(cards); i++ {
    txt += fmt.Sprintf("┌───────┐")
  }
  txt += "\n"

  for _, card := range cards {
    pad := " "

    if card.NumValue == 10 {
      pad = ""
    }

    txt += fmt.Sprintf("│ %s%s  │", card.Name, pad)
  }
  txt += "\n"

  for i := 0; i < len(cards); i++ {
    txt += fmt.Sprintf("└───────┘")
  }
  txt += "\n"

  return txt
}

func cliInputLoop(cli *CLI) {
  defer cli.app.Stop()

  for {
    select {
    case netData := <-cli.inputChan:
      if netData.NeedsTable() {
        assert(netData.Table != nil,
          fmt.Sprintf("%s: netData.Table == nil", netData.NetActionToString()))
      }
      if netData.NeedsPlayer() {
        assert(netData.Client.Player != nil,
          fmt.Sprintf("%s: Client.Player == nil", netData.NetActionToString()))
        // players always have an ID as well
        assert(netData.Client.ID != "",
          fmt.Sprintf("%v %s: ID empty", netData.Response,
                      netData.NetActionToString()))
      }

      switch netData.Response {
      case NetDataNewConn:
        if cli.yourClient.ID == "" {
          cli.yourClient = *netData.Client
          cli.settings = *netData.Client.Settings
        }
        cli.updateInfoList("# connected", netData.Table)
      case NetDataClientExited:
        cli.updateInfoList("# connected", netData.Table)
      case NetDataChatMsg:
        cli.updateChat(netData.Client, netData.Msg)
      case NetDataClientSettings:
        cli.yourClient = *netData.Client
        cli.settings = *netData.Client.Settings

        textViewSetLine(cli.yourInfoView, 1, "name: " + cli.yourClient.Name)
        nameInputField := cli.settingsForm.GetFormItem(0).(*tview.InputField)
        nameInputField.SetText(cli.settings.Name)

        if cli.yourClient.Player != nil {
          cli.playersTextViewMap[cli.yourClient.ID] = cli.yourInfoView
          cli.updatePlayer(&cli.yourClient, nil)
        }
      case NetDataYourPlayer:
        if netData.Table != nil {
          cli.updateInfoList("# players", netData.Table)
        }

        cli.yourClient = *netData.Client
        cli.settings = *cli.yourClient.Settings

        nameInputField := cli.settingsForm.GetFormItem(0).(*tview.InputField)
        nameInputField.SetText(cli.settings.Name)

        cli.playersTextViewMap[cli.yourClient.ID] = cli.yourInfoView

        cli.updatePlayer(&cli.yourClient, nil)
      case NetDataNewPlayer, NetDataCurPlayers:
        cli.updateInfoList("# players", netData.Table)

        cli.addNewPlayer(netData.Client)
      case NetDataPlayerLeft:
        cli.removePlayer(netData.Client)
        cli.updateInfoList("# players", netData.Table)
        cli.updateChat(nil, fmt.Sprintf("<server-msg> %s left the table",
                                       netData.Client.Player.Name))
      case NetDataMakeAdmin:
        cli.actionsForm.AddButton("start game", func() {
          cli.handleButton("start game")
        })

        tableLockKeys := make([]int, 0)
        for k := range TableLockNameMap {
          tableLockKeys = append(tableLockKeys, int(k))
        }
        sort.Ints(tableLockKeys)
        tableLockOpts := make([]string, 0)
        for _, lock := range tableLockKeys {
          tableLockOpts = append(tableLockOpts, TableLockNameMap[TableLock(lock)])
        }

        cli.settingsForm.AddTextView("admin options", "", 0, 1, false, false).
        AddDropDown("table lock", tableLockOpts, int(netData.Table.Lock),
          func(opt string, optIdx int) {
            lock := TableLock(optIdx)
            if _, ok := TableLockNameMap[lock]; ok {
              cli.settings.Admin.Lock = lock
            }
        }).
        AddInputField("table password", netData.Table.Password, 0, nil,
          func(pass string) {
            cli.settings.Admin.Password = pass
        })

        cli.settingsForm.GetFormItemByLabel("admin options").
           SetFormAttributes(0, tcell.ColorRed, tcell.ColorWhite,
                             tcell.ColorWhite, tcell.ColorWhite)

        cli.updateChat(nil, fmt.Sprintf("<server-msg> you are now the table admin"))

        cli.isTableAdmin = true
      case NetDataDeal:
        cli.commView.Clear()
        cli.clearPlayerScreens()

        cli.updatePlayer(netData.Client, netData.Table)

        txt := cli.cards2String(netData.Client.Player.Hole.Cards)

        cli.holeView.SetText(txt)
      case NetDataPlayerAction:
        cli.updatePlayer(netData.Client, netData.Table)
        cli.updateInfoList("status", netData.Table)
      case NetDataPlayerHead:
        if cli.playerHeadTextView != nil {
          if cli.playerHeadTextView == cli.curPlayerTextView {
            cli.playerHeadTextView.SetBorderColor(tcell.ColorRed)
          } else {
            cli.playerHeadTextView.SetBorderColor(tcell.ColorWhite)
          }
        }
        if netData.Client == nil { // clear head request
          cli.app.Draw()
          continue
        }
        if playerHeadTextView, ok := cli.playersTextViewMap[netData.Client.ID]; ok {
          playerHeadTextView.SetBorderColor(tcell.ColorOrange)
          cli.playerHeadTextView = playerHeadTextView
        }
      case NetDataPlayerTurn:
        curPlayerTextView := cli.playersTextViewMap[netData.Client.ID]

        if curPlayerTextView == nil {
          continue // XXX probably would be a bug
        }

        if cli.curPlayerTextView != nil {
          if cli.curPlayerTextView == cli.playerHeadTextView {
            cli.curPlayerTextView.SetBorderColor(tcell.ColorOrange)
          } else {
            cli.curPlayerTextView.SetBorderColor(tcell.ColorWhite)
          }
        }

        cli.curPlayerTextView = curPlayerTextView

        cli.curPlayerTextView.SetBorderColor(tcell.ColorRed)

        // set focus in case the user was focused on chat
        if curPlayerTextView == cli.yourInfoView {
          if page, _ := cli.pages.GetFrontPage(); page == "game" {
            cli.app.SetFocus(cli.actionsForm)
          }
        }
      case NetDataUpdatePlayer:
        cli.updatePlayer(netData.Client, netData.Table)
      case NetDataUpdateTable:
        cli.updateInfoList("status", netData.Table)
      case NetDataCurHand:
        cli.updatePlayer(netData.Client, netData.Table)
      case NetDataShowHand:
        cli.updatePlayer(netData.Client, nil)
      case NetDataRoundOver:
        cli.updateInfoList("status", netData.Table)
        cli.errorModal.SetText(netData.Msg)
        cli.switchToPage("error")

        //for _, player := range netData.Table.Winners {
        //  cli.updatePlayer(netData.Client, nil)
        //}
      case NetDataReset:
        if netData.Client != nil && netData.Client.Player != nil {
          for name, textView := range cli.playersTextViewMap {
            if name != netData.Client.Name {
              cli.otherPlayersFlex.RemoveItem(textView)
            }
          }
        } else {
          for _, textView := range cli.playersTextViewMap {
            cli.otherPlayersFlex.RemoveItem(textView)
          }
        }
        cli.holeView.Clear()
        cli.updateInfoList("all", netData.Table)
      case NetDataEliminated:
        if netData.Client.ID == cli.yourClient.ID {
          cli.unmakeAdmin()
          cli.errorModal.SetText("you were eliminated")
          cli.yourInfoView.Clear()
          cli.holeView.Clear()
          cli.clearPlayerScreens()
          //textViewSetLine(cli.yourInfoView, 1, "name: " + cli.yourClient.Name)
          cli.yourInfoView.SetText("name: " + cli.yourClient.Name)
          cli.switchToPage("error")
        }

        cli.updateChat(netData.Client, netData.Msg)
      case NetDataFlop, NetDataTurn, NetDataRiver:
        txt := cli.cards2String(netData.Table.Community)

        cli.commView.SetText(txt)
        cli.updateInfoList("status", netData.Table)
      case NetDataBadRequest, NetDataServerMsg:
        if netData.Msg == "" {
          if netData.Response == NetDataBadRequest {
            netData.Msg = "unspecified server error"
          } else {
            netData.Msg = "BUG: empty server message"
          }
        }

        cli.errorModal.SetText(netData.Msg)
        cli.switchToPage("error")
      case NetDataTableLocked, NetDataBadAuth:
        cli.finish <- errors.New(netData.Msg)
      case NetDataServerClosed:
        cli.finish <- errors.New("server closed")
      default:
        cli.finish <- errors.New("bad response")
      }

      cli.app.Draw()
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

  if err := cli.app.SetRoot(cli.pages, true).SetFocus(cli.actionsFlex).Run(); err != nil {
    return err
  }

  //fmt.Println(postOut)

  return nil
}
