package poker

import (
	"fmt"

	"github.com/bkazemi/gopoker/internal/playerState"
)

type Action struct {
  Action playerState.PlayerState
  Amount Chips
}

func (action *Action) Clear() {
  action.Action = playerState.FirstAction
  action.Amount = 0
}

type Player struct {
  defaultName string
  Name        string // NOTE: must have unique names
  IsCPU       bool

  IsVacant bool
  TablePos uint

  ChipCount Chips
  Hole      *Hole
  Hand      *Hand
  PreHand   Hand // XXX tmp
  Action    Action
}

func (p *Player) SetName(name string) {
  oldName := p.Name
  if name == "" {
    p.Name = p.defaultName
  } else {
    p.Name = name
  }
  fmt.Printf("Player.setName(): <%s> '%s' => '%s'\n", p.defaultName, oldName, p.Name)
}

func (p *Player) canBet() bool {
  return !p.IsVacant && p.Action.Action != playerState.MidroundAddition &&
         p.Action.Action != playerState.Fold && p.Action.Action != playerState.AllIn
}

// XXX: should consider adding an ~IsBlind field to Player struct
func (p *Player) isABlind(table *Table) bool {
  if table == nil {
    panic("Player.isABlind(): table == nil")
  }

  return ((table.BigBlind   != nil && table.BigBlind.Player.Name   == p.Name) ||
          (table.SmallBlind != nil && table.SmallBlind.Player.Name == p.Name))
}

type PlayerNode struct {
  /*prev,*/ next *PlayerNode // XXX don't think i need this to be a pointer. check back
  Player *Player
}

// NOTE: the next field needs a getter/setter because gob/msgpack cannot handle
//       circular references.

func (node *PlayerNode) Next() *PlayerNode {
  return node.next
}

func (node *PlayerNode) SetNext(next *PlayerNode) {
  node.next = next
}

// circular list of players at the poker table
//
// NOTE: _never_ export this field to something that needs to be encoded for
//       reason stated above
type PlayerList struct {
  Len int
  Name string
  Head *PlayerNode // XXX don't think i need this to be a pointer. check back
}

func NewPlayerList(name string, players []*Player) *PlayerList {
  list := &PlayerList{
    Name: name,
  }

  if players == nil || len(players) == 0 {
    fmt.Printf("NewPlayerList(): <%s> called with empty player list\n", name)

    return list
  }

  list.Head = &PlayerNode{Player: players[0]}
  head := list.Head
  if len(players) > 1 {
    for _, p := range players[1:] {
      list.Head.SetNext(&PlayerNode{Player: p})
      list.Head = list.Head.Next()
    }
  } else {
    fmt.Printf("NewPlayerList(): <%s> called with len(players) == 1\n", name)
  }
  list.Head.SetNext(head)
  list.Head = head
  list.Len = len(players)

  return list
}

func (list *PlayerList) Print() {
  fmt.Printf("<%s> len==%v [ ", list.Name, list.Len)
  for i, n := 0, list.Head; i < list.Len; i++ {
    fmt.Printf("%s n=> %s ", n.Player.Name, n.Next().Player.Name)
    n = n.Next()
    if i == list.Len-1 {
      fmt.Printf("| n=> %s ", n.Next().Player.Name)
    }
  }
  fmt.Println("]")
}

func (list *PlayerList) Clone(newName string) *PlayerList {
  return NewPlayerList(newName, list.ToPlayerArray())
}

func (list *PlayerList) AddPlayer(player *Player) {
  if list.Len == 0 {
    list.Head = &PlayerNode{Player: player}
    list.Head.SetNext(list.Head)
  } else {
    newNode := &PlayerNode{Player: player, next: list.Head}

    node := list.Head
    for i := 0; i < list.Len - 1; i++ {
      node = node.Next()
    }
    node.SetNext(newNode)
  }

  list.Len++
}

func (list *PlayerList) RemovePlayer(player *Player) *PlayerNode {
  if list.Len == 0 || player == nil {
    return nil
  }

  fmt.Printf("&PlayerList.RemovePlayer(): <%s> called for %s\n", list.Name, player.Name)
  fmt.Printf("&PlayerList.RemovePlayer(): was ") ; list.Print()

  foundPlayer := true

  defer func() {
    if foundPlayer {
      list.Len--
    }
    fmt.Printf("&PlayerList.RemovePlayer(): now ") ; list.Print()
  }()

  node, prevNode := list.Head, list.Head
  for i := 0; i < list.Len; i++ {
    if node.Player.Name == player.Name {
      if i == 0 {
        if list.Len == 1 {
          list.Head = nil
          return nil
        }

        list.Head = list.Head.Next()

        tailNode := list.Head
        for j := 0; j < list.Len-2; j++ {
          tailNode = tailNode.Next()
        }
        tailNode.SetNext(list.Head)

        return list.Head
      } else {
        prevNode.SetNext(node.Next())

        return prevNode.Next()
      }
    }

    prevNode = node
    node = node.Next()
  }

  fmt.Printf("&PlayerList.RemovePlayer(): <%s> %s not found in list\n", list.Name, player.Name)

  foundPlayer = false
  return nil // player not found
}

func (list *PlayerList) GetPlayerNode(player *Player) *PlayerNode {
  node := list.Head

  //fmt.Printf("&PlayerList.GetPlayerNode(): called for %s\n", player.Name)
  //list.ToNodeArray()

  for i := 0; i < list.Len; i++ {
    if node.Player.Name == player.Name {
      return node
    }
    node = node.Next()
  }

  return nil
}

func (list *PlayerList) SetHead(node *PlayerNode) {
  if node == nil {
    fmt.Printf("%s.SetHead(): setting parameter is nil\n", list.Name)
    if list.Len != 0 {
      fmt.Printf(" with a nonempty list\n")
    } else {
      fmt.Println()
    }
  }

  list.Head = node
}

func (list *PlayerList) ToNodeArray() []*PlayerNode {
  nodes := make([]*PlayerNode, 0)

  for i, node := 0, list.Head; i < list.Len; i++ {
    nodes = append(nodes, node)
    node = node.Next()
  }

  //fmt.Printf("&PlayerList.ToNodeArray(): ") ; list.Print()

  return nodes
}

func (list *PlayerList) ToPlayerArray() []*Player {
  if list.Len == 0 {
    return nil
  }

  players := make([]*Player, 0)

  for i, node := 0, list.Head; i < list.Len; i++ {
    players = append(players, node.Player)
    node = node.Next()
  }

  //fmt.Printf("&PlayerList.ToPlayerArray(): ") ; list.Print()

  return players
}

func NewPlayer(name string, isCPU bool) *Player {
  player := &Player{
    defaultName: name,
    Name: name,
    IsCPU: isCPU,

    IsVacant: true,

    ChipCount: 1e5, // TODO: add knob

    Action: Action{Action: playerState.VacantSeat},
  }

  player.NewCards()

  return player
}

func (player *Player) NewCards() {
  player.Hole = &Hole{Cards: make(Cards, 0, 2)}
  player.Hand = &Hand{Rank: RankMuck, Cards: make(Cards, 0, 5)}
}

func (player *Player) Clear() {
  player.Name = player.defaultName
  player.IsVacant = true

  player.ChipCount = 1e5 // XXX
  player.NewCards()

  player.Action.Amount = 0
  player.Action.Action = playerState.VacantSeat
}

func (player *Player) ChipCountToString() string {
  return printer.Sprintf("%d", player.ChipCount)
}

func (player *Player) ActionToString() string {
  switch player.Action.Action {
  case playerState.AllIn:
    return printer.Sprintf("all in (%d chips)", player.Action.Amount)
  case playerState.Bet:
    return printer.Sprintf("raise (bet %d chips)", player.Action.Amount)
  case playerState.Call:
    return printer.Sprintf("call (%d chips)", player.Action.Amount)
  case playerState.Check:
    return "check"
  case playerState.Fold:
    return "fold"

  case playerState.VacantSeat:
    return "seat is open" // XXX
  case playerState.PlayerTurn:
    return "(player's turn) waiting for action"
  case playerState.FirstAction:
    return "waiting for first action"
  case playerState.MidroundAddition:
    return "waiting to add to next round"

  default:
    return "bad player state"
  }
}

