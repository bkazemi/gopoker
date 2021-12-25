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
  holeView          *tview.TextView

  actionsForm       *tview.Form

  infoList          *tview.List

  exitModal         *tview.Modal

  inputChan          chan *NetData
  outputChan         chan *NetData

  finish             chan error
}

func (cli *CLI) eventHandler(eventKey *tcell.EventKey) *tcell.EventKey {
  if eventKey.Rune() == 'q' {
    cli.pages.SwitchToPage("exit")
  }

  if eventKey.Rune() == 'b' {

  }

  return eventKey
}

func (cli *CLI) Init() error {
  cli.app        = tview.NewApplication()
  cli.pages      = tview.NewPages()
  cli.gameGrid   = tview.NewGrid()
  cli.exitModal  = tview.NewModal()
  cli.inputChan  = make(chan *NetData)
  cli.outputChan = make(chan *NetData)
  cli.finish     = make(chan error)

  newTextView := func(title string, save **tview.TextView) *tview.TextView {
    ret := tview.NewTextView()//.
           //SetChangedFunc(func() {
           //  cli.app.Draw() // XXX wrong func?
           //})

    if save != nil {
      *save = ret
    }

    ret.SetBorder(true).SetTitle(title)
    
    return ret
  }

  cli.actionsForm = tview.NewForm().
    AddInputField("bet amount", "", 20, nil, nil).
    AddButton("call",  nil).
    AddButton("raise", nil).
    AddButton("fold",  nil).
    AddButton("quit",  nil)
  cli.actionsForm.SetBorder(true).SetTitle("Actions")

  cli.infoList = tview.NewList().
    AddItem("# players",   "", '-', nil).
    AddItem("# connected", "", '-', nil).
    AddItem("buy in",      "", '-', nil).
    AddItem("big blind",   "", '-', nil).
    AddItem("small blind", "", '-', nil)
  cli.infoList.SetBorder(true).SetTitle("Table Info")

  cli.gameGrid.
    SetRows(0, 0, 0).
    SetColumns(0, 0, 0).
    AddItem(newTextView("Community Cards", &cli.commView),         0, 0, 1, 3, 0, 0, false).
    AddItem(newTextView("Other Players",   &cli.otherPlayersView), 1, 0, 1, 3, 0, 0, false).
    AddItem(cli.actionsForm,                                       2, 0, 1, 1, 0, 0, false).
    AddItem(newTextView("Hole",            &cli.holeView),         2, 1, 1, 1, 0, 0, false).
    AddItem(cli.infoList,                                          2, 2, 1, 1, 0, 0, false)

  cli.exitModal.SetText("do you want to quit the game?").
    AddButtons([]string{"quit", "cancel"}).
    SetDoneFunc(func(btnIdx int, btnLabel string) {
      switch btnLabel {
      case "quit":
        cli.app.Stop()
      case "cancel":
        cli.pages.SwitchToPage("game")
      }
    })

  cli.pages.AddPage("game", cli.gameGrid, true, true)
  cli.pages.AddPage("exit", cli.exitModal, true, false)

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

func (cli *CLI) Run() error {
  go func() {
    for {
      select {
      case netData := <-cli.inputChan:
        switch netData.Response {
        case NETDATA_NEWCONN, NETDATA_CLIENTEXITED:
          cli.infoList.SetItemText(1, "# connected", strconv.FormatUint(uint64(netData.Table.NumConnected), 10))
        case NETDATA_STARTGAME:
          ;
        case NETDATA_DOFLOP:
          txt := "\n"
          for _, card := range netData.Table.Community {
            txt += fmt.Sprintf(" [%s]", card.Name)
          }
          cli.commView.SetText(txt)
        default:
          panic("...")
        }

      case err := <-cli.finish:
        if err != nil {
          fmt.Printf("backend error: %s\n", err)
        }
        cli.app.Stop()
        return
      }
    }
  }()

  if err := cli.app.SetRoot(cli.pages, true).SetFocus(cli.pages).Run(); err != nil {
    return err
	}

  return nil
}
