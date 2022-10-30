package main

import (
	"flag"
	"fmt"

	//"net"
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"

	//"io"
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	//_ "net/http/pprof"

	crypto_rand "crypto/rand"
	math_rand "math/rand"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/gorilla/websocket"
)

// ranks
const (
  R_MUCK = iota - 1
  R_HIGHCARD
  R_PAIR
  R_2PAIR
  R_TRIPS
  R_STRAIGHT
  R_FLUSH
  R_FULLHOUSE
  R_QUADS
  R_STRAIGHTFLUSH
  R_ROYALFLUSH
)

// cards
const (
  C_ACELOW = iota + 1
  C_TWO
  C_THREE
  C_FOUR
  C_FIVE
  C_SIX
  C_SEVEN
  C_EIGHT
  C_NINE
  C_TEN
  C_JACK
  C_QUEEN
  C_KING
  C_ACE
)

// suits
const (
  S_CLUB = iota
  S_DIAMOND
  S_HEART
  S_SPADE
)

type Card struct {
  Name     string
  FullName string
  Suit     int
  NumValue int // numeric value of card
}

type Cards []*Card

type Hole struct {
  IsSuited         bool
  Suit             int
  IsPair           bool
  CombinedNumValue int
  Cards            Cards
}

func (hole *Hole) FillHoleInfo() {
  if hole.Cards[0].NumValue == hole.Cards[1].NumValue {
    hole.IsPair = true
  }

  if hole.Cards[0].Suit == hole.Cards[1].Suit {
    hole.IsSuited = true
    hole.Suit = hole.Cards[0].Suit
  }

  hole.CombinedNumValue = hole.Cards[0].NumValue + hole.Cards[1].NumValue
}

type Hand struct {
  Rank   int
  Kicker int
  Cards  Cards
}

func (hand *Hand) RankName() string {
  rankNameMap := map[int]string{
    R_MUCK:          "muck",
    R_HIGHCARD:      "high card",
    R_PAIR:          "pair",
    R_2PAIR:         "two pair",
    R_TRIPS:         "three of a kind",
    R_STRAIGHT:      "straight",
    R_FLUSH:         "flush",
    R_FULLHOUSE:     "full house",
    R_QUADS:         "four of a kind",
    R_STRAIGHTFLUSH: "straight flush",
  }

  if rankName, ok := rankNameMap[hand.Rank]; ok {
    return rankName
  }

  panic("RankName(): BUG")
}

type Action struct {
  Action int
  Amount uint
}

type Player struct {
  Name  string // NOTE: must have unique names
  IsCPU bool

  IsVacant bool

  ChipCount uint
  Hole      *Hole
  Hand      *Hand
  PreHand   Hand // XXX tmp
  Action    Action
}

func (p *Player) canBet() bool {
  return !p.IsVacant && p.Action.Action != NETDATA_MIDROUNDADDITION &&
         p.Action.Action != NETDATA_FOLD && p.Action.Action != NETDATA_ALLIN
}

type PlayerNode struct {
  /*prev,*/ next *PlayerNode // XXX don't think i need this to be a pointer. check back
  Player *Player
}

// circular list of players at the poker table
type playerList struct {
  len int
  name string
  node *PlayerNode // XXX don't think i need this to be a pointer. check back
}

func (list *playerList) Init(name string, players []*Player) error {
  if len(players) < 2 {
    return errors.New("playerList(): Init(): players param must be >= 2")
  }

  list.name = name

  list.node = &PlayerNode{Player: players[0]}
  head := list.node
  for _, p := range players[1:] {
    list.node.next = &PlayerNode{Player: p}
    list.node = list.node.next
  }
  list.node.next = head
  list.node = head
  list.len = len(players)

  return nil
}

func (list *playerList) Clone(newName string) playerList {
  if list.len == 0 {
    return playerList{}
  }

  if list.len == 1 {
    clonedList := playerList{name: newName, node: &PlayerNode{Player: list.node.Player}, len: 1}
    clonedList.node.next = clonedList.node

    return clonedList
  }

  clonedList := playerList{}
  clonedList.Init(newName, list.ToPlayerArray())

  return clonedList
}

func (list *playerList) AddPlayer(player *Player) {
  if list.len == 0 {
    list.node = &PlayerNode{Player: player}
    list.node.next = list.node
  } else {
    newNode := &PlayerNode{Player: player, next: list.node}

    node := list.node
    for i := 0; i < list.len - 1; i++ {
      node = node.next
    }
    node.next = newNode
  }

  list.len++
}

func (list *playerList) RemovePlayer(player *Player) *PlayerNode {
  if list.len == 0 || player == nil {
    return nil
  }

  fmt.Printf("RemovePlayer(): <%s> called for %s\n", list.name, player.Name)

  fmt.Printf("RemovePlayer(): was len==%v [", list.len)
  for i, n := 0, list.node; i<list.len;i++{
    fmt.Printf(" %s n=> %s ", n.Player.Name, n.next.Player.Name)
    n=n.next
    if i == list.len-1 {
      fmt.Printf(" | n=> %s ", n.next.Player.Name)
    }
  };fmt.Println("]")

  foundPlayer := true

  defer func() {
    if foundPlayer {
      list.len--
    }
    fmt.Printf("RemovePlayer(): now len==%v [", list.len)
    for i, n := 0, list.node; i<list.len;i++{
      fmt.Printf(" %s n=> %s ", n.Player.Name, n.next.Player.Name)
      n=n.next
      if i == list.len-1 {
        fmt.Printf(" | n=> %s ", n.next.Player.Name)
      }
    };fmt.Println("]")
  }()

  node, prevNode := list.node, list.node
  for i := 0; i < list.len; i++ {
    if node.Player.Name == player.Name {
      if i == 0 {
        if list.len == 1 {
          list.node = nil
          return nil
        }

        list.node = list.node.next

        tailNode := list.node
        for j := 0; j < list.len-2; j++ {
          tailNode = tailNode.next
        }
        tailNode.next = list.node

        return list.node
      } else {
        prevNode.next = node.next

        return prevNode.next
      }
    }

    prevNode = node
    node = node.next
  }

  fmt.Printf("RemovePlayer(): %s not found in list\n", player.Name)

  foundPlayer = false
  return nil // player not found
}

func (list *playerList) GetPlayerNode(player *Player) *PlayerNode {
  node := list.node

  fmt.Printf("GetPlayerNode(): called for %s\n", player.Name)
  list.ToNodeArray()

  for i := 0; i < list.len; i++ {
    if node.Player.Name == player.Name {
      return node
    }
    node = node.next
  }

  return nil
}

func (list *playerList) ToNodeArray() []*PlayerNode {
  nodes := make([]*PlayerNode, 0)

  for i, node := 0, list.node; i < list.len; i++ {
    nodes = append(nodes, node)
    node = node.next
  }

  fmt.Printf("tNA: <%s> len==%v [", list.name, list.len)
  for i, n := 0, list.node; i < list.len; i++ {
    if n == nil || n.Player == nil {
      fmt.Printf(" <idx %v p is nil> ", i)
      continue
    }
    fmt.Printf(" %s n=> %s", n.Player.Name, n.next.Player.Name)
    n = n.next
    if i == list.len-1 {
      fmt.Printf(" | n=> %s ", n.next.Player.Name)
    }
  };fmt.Println("]")


  return nodes
}


func (list *playerList) ToPlayerArray() []*Player {
  if list.len == 0 {
    return nil
  }

  players := make([]*Player, 0)

  for i, node := 0, list.node; i < list.len; i++ {
    players = append(players, node.Player)
    node = node.next
  }

  fmt.Printf("tPA: <%s> len==%v [", list.name, list.len)
  for i, n := 0, list.node; i < list.len; i++ {
    if n == nil || n.Player == nil {
      fmt.Printf(" <idx %v p is nil> ", i)
      continue
    }
    fmt.Printf(" %s n=> %s", n.Player.Name, n.next.Player.Name)
    n = n.next
    if i == list.len-1 {
      fmt.Printf(" | n=> %s ", n.next.Player.Name)
    }
  };fmt.Println("]")

  return players
}

func (player *Player) Init(name string, isCPU bool) error {
  player.Name = name
  player.IsCPU = isCPU

  player.IsVacant = true

  player.ChipCount = 1e5 // XXX
  player.NewCards()

  player.Action = Action{Action: NETDATA_VACANTSEAT}

  return nil
}

func (player *Player) NewCards() {
  player.Hole = &Hole{Cards: make(Cards, 0, 2)}
  player.Hand = &Hand{Rank: R_MUCK, Cards: make(Cards, 0, 5)}
}

func (player *Player) Clear() {
  player.IsVacant = true

  player.ChipCount = 1e5 // XXX
  player.NewCards()

  player.Action.Amount = 0
  player.Action.Action = NETDATA_VACANTSEAT
}

func (player *Player) ChipCountToString() string {
  return printer.Sprintf("%d", player.ChipCount)
}

func (player *Player) ActionToString() string {
  switch player.Action.Action {
  case NETDATA_ALLIN:
    return printer.Sprintf("all in (%d chips)", player.Action.Amount)
  case NETDATA_BET:
    return printer.Sprintf("raise (bet %d chips)", player.Action.Amount)
  case NETDATA_CALL:
    return printer.Sprintf("call (%d chips)", player.Action.Amount)
  case NETDATA_CHECK:
    return "check"
  case NETDATA_FOLD:
    return "fold"

  case NETDATA_VACANTSEAT:
    return "seat is open" // XXX
  case NETDATA_PLAYERTURN:
    return "(player's turn) waiting for action"
  case NETDATA_FIRSTACTION:
    return "waiting for first action"
  case NETDATA_MIDROUNDADDITION:
    return "waiting to add to next round"

  default:
    return "bad player state"
  }
}

type Deck struct {
  pos   uint
  cards Cards
}

func (deck *Deck) Init() error {
  deck.cards = make(Cards, 52, 52) // 52 cards in a poker deck

  for suit := S_CLUB; suit <= S_SPADE; suit++ {
    for c_num := C_TWO; c_num <= C_ACE; c_num++ {
      curCard := &Card{Suit: suit, NumValue: c_num}
      if err := cardNumToString(curCard); err != nil {
        return err
      }

      deck.cards[deck.pos] = curCard
      deck.pos++
    }
  }

  deck.pos = 0

  return nil
}

func (deck *Deck) Shuffle() {
  for i := math_rand.Intn(4) + 1; i > 0; i-- {
    for i := 0; i < 52; i++ {
      randidx := math_rand.Intn(52)
      // swap
      deck.cards[randidx], deck.cards[i] = deck.cards[i], deck.cards[randidx]
    }
  }

  deck.pos = 0
}

// "remove" card from deck (functionally)
func (deck *Deck) Pop() *Card {
  deck.pos++
  return deck.cards[deck.pos-1]
}

type TableState int

const (
  TABLESTATE_NOTSTARTED TableState = iota

  TABLESTATE_PREFLOP
  TABLESTATE_FLOP
  TABLESTATE_TURN
  TABLESTATE_RIVER

  TABLESTATE_ROUNDS
  TABLESTATE_PLAYERRAISED
  TABLESTATE_DONEBETTING

  TABLESTATE_SHOWHANDS
  TABLESTATE_SPLITPOT
  TABLESTATE_ROUNDOVER
  TABLESTATE_NEWROUND
  TABLESTATE_GAMEOVER
  TABLESTATE_RESET
)

type Pot struct {
  Bet      uint
  Pot      uint
  Players  map[string]*Player
  IsClosed bool
  WinInfo  string
}

type SidePot Pot

func (sidePot *SidePot) Init(playerMap map[string]*Player, player *Player) {
  if playerMap == nil && player == nil {
    sidePot.Players = make(map[string]*Player, 0)

    return
  }

  if playerMap != nil {
    sidePot.Players = make(map[string]*Player, len(playerMap)+1)

    for pName, p := range playerMap {
      sidePot.Players[pName] = p
    }

    if player != nil {
      if p := sidePot.Players[player.Name]; p == nil {
        sidePot.Players[player.Name] = player
      }
    }
  } else if player != nil {
    sidePot.Players = make(map[string]*Player, 1)

    sidePot.Players[player.Name] = player
  }
}

type Table struct {
  deck       *Deck // deck of cards
  Community  Cards // community cards
  _comsorted Cards // sorted community cards

  MainPot *Pot       // table pot
  SidePot []*SidePot // sidepots for allins
  Ante    uint       // current ante TODO allow both ante & blind modes
  Bet     uint       // current bet

  Dealer     *PlayerNode // current dealer
  SmallBlind *PlayerNode // current small blind
  BigBlind   *PlayerNode // current big blind

  players       []*Player   // array of all seats at table
  activePlayers playerList  // list of all active players
  curPlayers    playerList  // list of actively betting players (no folders or all-ins)
  Winners       []*Player   // array of round winners
  curPlayer     *PlayerNode // keeps track of whose turn it is
  better        *Player     // last player to (re-)raise XXX currently unused
  NumPlayers    uint        // number of current players
  NumSeats      uint        // number of total possible players
  roundCount    uint        // total number of rounds played

  WinInfo string // XXX tmp

  State        TableState // current status of table
  CommState    TableState // current status of community
  NumConnected uint       // number of people (players+spectators) currently at table (online mode)

  mtx sync.Mutex
}

func (table *Table) Init(deck *Deck, CPUPlayers []bool) error {
  table.deck = deck
  table.Ante = 10

  table.newCommunity()

  table.MainPot = &Pot{}

  table.SidePot = make([]*SidePot, 0)

  table.players = make([]*Player, 0, table.NumSeats) // 2 players min, 7 max
  table.activePlayers = playerList{name: "activePlayers"}
  table.curPlayers = playerList{name: "curPlayers"}

  if table.NumSeats < 2 {
    return errors.New("need at least two players")
  }

  for i := uint(0); i < table.NumSeats; i++ {
    player := &Player{}
    if err := player.Init(fmt.Sprintf("p%d", i), CPUPlayers[i]); err != nil {
      return err
    }

    table.players = append(table.players, player)
  }

  //table.curPlayers = table.players

  table.Dealer = nil
  table.SmallBlind = nil
  table.BigBlind = nil

  return nil
}

func (table *Table) reset(player *Player) {
  table.mtx.Lock()
  defer table.mtx.Unlock()

  fmt.Println("resetting table")

  table.Ante = 10

  table.newCommunity()

  for _, node := range table.curPlayers.ToNodeArray() {
    if player == nil || player.Name != node.Player.Name {
      fmt.Printf("reset(): cleared %s\n", node.Player.Name)
      node.Player.Clear()
      table.curPlayers.RemovePlayer(node.Player)
    } else {
      fmt.Printf("reset(): skipped %s\n", node.Player.Name)
      table.curPlayers.node = node

      player.NewCards()
      player.Action.Action, player.Action.Amount = NETDATA_FIRSTACTION, 0
    }
  }

  table.Winners, table.better = nil, nil

  if table.curPlayers.len == 0 && player != nil {
    fmt.Printf("reset(): curPlayers was empty, adding winner %s\n", player.Name)
    table.curPlayers.AddPlayer(player)
  } else {
    table.curPlayer = table.curPlayers.node
  }

  // XXX
  for _, p := range table.players {
    if player == nil || p.Name != player.Name {
      p.Clear()
    }
  }

  table.MainPot = &Pot{}

  table.Bet, table.NumPlayers, table.roundCount = 0, 0, 0

  if player != nil {
    table.NumPlayers++
  }

  table.WinInfo = ""

  table.State = TABLESTATE_NOTSTARTED

  table.Dealer = table.activePlayers.node

  table.SmallBlind = nil
  table.BigBlind = nil
}

func (table *Table) newCommunity() {
  table.Community = make(Cards, 0, 5)
  table._comsorted = make(Cards, 0, 5)
}

func (table *Table) CommunityToString() string {
  comm := ""

  for _, card := range table.Community {
    comm += fmt.Sprintf("[%s] ", card.Name)
  }

  return comm
}

func (table *Table) TableStateToString() string {
  tableStateNameMap := map[TableState]string{
    TABLESTATE_NOTSTARTED: "waiting for start",

    TABLESTATE_PREFLOP: "preflop",
    TABLESTATE_FLOP:    "flop",
    TABLESTATE_TURN:    "turn",
    TABLESTATE_RIVER:   "river",

    TABLESTATE_ROUNDS:    "betting rounds",
    TABLESTATE_ROUNDOVER: "round over",
    TABLESTATE_NEWROUND:  "new round",
    TABLESTATE_GAMEOVER:  "game over",

    TABLESTATE_PLAYERRAISED: "player raised",
    TABLESTATE_DONEBETTING:  "finished betting",
    TABLESTATE_SHOWHANDS:    "showing hands",
    TABLESTATE_SPLITPOT:     "split pot",
  }

  if state, ok := tableStateNameMap[table.State]; ok {
    return state
  }

  return "BUG: bad table state"
}

func (table *Table) commState2NetDataResponse() int {
  commStateNetDataMap := map[TableState]int{
    TABLESTATE_FLOP:  NETDATA_FLOP,
    TABLESTATE_TURN:  NETDATA_TURN,
    TABLESTATE_RIVER: NETDATA_RIVER,
  }

  if netDataResponse, ok := commStateNetDataMap[table.CommState]; ok {
    return netDataResponse
  }

  fmt.Printf("commState2NetDataResponse(): bad state `%v`\n", table.CommState)
  return NETDATA_BADREQUEST
}

func (table *Table) PotToString() string {
  return printer.Sprintf("%d chips", table.MainPot.Pot)
}

func (table *Table) BigBlindToString() string {
  if table.BigBlind != nil {
    return printer.Sprintf("%s (%d chip bet)", table.BigBlind.Player.Name, table.Ante)
  }

  return "none"
}

func (table *Table) DealerToString() string {
  if table.Dealer != nil {
    return table.Dealer.Player.Name
  }

  return "none"
}

func (table *Table) SmallBlindToString() string {
  if table.SmallBlind != nil {
    return printer.Sprintf("%s (%d chip bet)", table.SmallBlind.Player.Name, table.Ante/2)
  }

  return "none"
}

func (table *Table) PublicPlayerInfo(player Player) *Player {
  if table.State != TABLESTATE_SHOWHANDS {
    player.Hole, player.Hand = nil, nil
  }

  return &player
}

func (table *Table) allInCount() int {
  allIns := 0

  for _, p := range table.activePlayers.ToPlayerArray() {
    fmt.Printf("aiCnt: pa: %v\n", p.ActionToString())
    if p.Action.Action == NETDATA_ALLIN {
      allIns++
    }
  }

  return allIns
}

func (table *Table) bettingIsImpossible() bool {
  // only <= 1 player(s) has any chips left to bet
  return table.curPlayers.len < 2
}

func (table *Table) closeSidePots() {
  if len(table.SidePot) == 0 {
    return
  }

  if !table.MainPot.IsClosed {
    fmt.Printf("closing mainpot\n")
    table.MainPot.IsClosed = true
  }

  for i, sidePot := range table.SidePot[:len(table.SidePot)-1] {
    if !sidePot.IsClosed {
      fmt.Printf("closing sidepot #%d\n", i)
      sidePot.IsClosed = true
    }
  }
}

func (table *Table) getChipLeaders() (uint, uint) {
  if table.curPlayers.len < 2 {
    panic("BUG: getChipLeaders() called with < 2 non-folded/allin players")
  }

  var (
    chipLeader       uint = 0
    secondChipLeader uint = 0
  )

  for _, p := range table.curPlayers.ToPlayerArray() {
    if p.ChipCount > chipLeader {
      chipLeader = p.ChipCount
    }
  }

  for _, p := range table.curPlayers.ToPlayerArray() {
    if p.ChipCount != chipLeader && p.ChipCount > secondChipLeader {
      secondChipLeader = p.ChipCount
    }
  }

  return chipLeader, secondChipLeader
}

func (table *Table) getOpenSeat() *Player {
  table.mtx.Lock()
  defer table.mtx.Unlock()

  for _, seat := range table.players {
    if seat.IsVacant {
      seat.IsVacant = false
      table.NumPlayers++

      return seat
    }
  }

  return nil
}

func (table *Table) getOccupiedSeats() []*Player {
  seats := make([]*Player, 0)

  for _, seat := range table.players {
    if !seat.IsVacant {
      seats = append(seats, seat)
    }
  }

  return seats
}

func (table *Table) getUnoccupiedSeats() []*Player {
  seats := make([]*Player, 0)

  for _, seat := range table.players {
    if seat.IsVacant {
      seats = append(seats, seat)
    }
  }

  return seats
}

func (table *Table) getActiveSeats() []*Player {
  seats := make([]*Player, 0)

  for _, seat := range table.players {
    if !seat.IsVacant &&
      seat.Action.Action != NETDATA_MIDROUNDADDITION {
      seats = append(seats, seat)
    }
  }

  return seats
}

func (table *Table) GetNumOpenSeats() uint {
  return table.NumSeats - table.NumPlayers
}

func (table *Table) addNewPlayers() {
  for _, player := range table.activePlayers.ToPlayerArray() {
    if player.Action.Action == NETDATA_MIDROUNDADDITION {
      fmt.Printf("adding new player: %s\n", player.Name)
      player.Action.Action = NETDATA_FIRSTACTION
    }
  }
}

func (table *Table) getEliminatedPlayers() []*Player {
  table.mtx.Lock()
  defer table.mtx.Unlock()

  ret := make([]*Player, 0)

  for _, player := range table.activePlayers.ToPlayerArray() {
    if player.ChipCount == 0 {
      ret = append(ret, player)
    }
  }

  if uint(len(ret)) == table.NumPlayers-1 {
    table.State = TABLESTATE_GAMEOVER
  }

  fmt.Printf("getEliminatedPlayers(): [")
  for _, p := range ret {
    fmt.Printf(" %s ", p.Name)
  }; fmt.Println("]")

  return ret
}

// resets the active players list head to
// Bb+1 pre-flop
// Sb post-flop
func (table *Table) reorderPlayers() {
  if table.State == TABLESTATE_NEWROUND ||
    table.State == TABLESTATE_PREFLOP {
    table.activePlayers.node = table.BigBlind.next
    table.curPlayers.node = table.curPlayers.GetPlayerNode(table.BigBlind.next.Player)
    assert(table.curPlayers.node != nil, "reorderPlayers(): couldn't find Bb+1 player node")
    fmt.Printf("reorderPlayers(): curPlayers head now: %s\n", table.curPlayers.node.Player.Name)
  } else { // post-flop
    smallBlindNode := table.SmallBlind
    if smallBlindNode == nil { // smallblind left mid game
      if table.Dealer != nil {
        smallBlindNode = table.Dealer.next
      } else if table.BigBlind != nil {
        smallBlindNode = table.activePlayers.node
        // definitely considering doubly linked lists now *sigh*
        for smallBlindNode.next.Player.Name != table.BigBlind.Player.Name {
          smallBlindNode = smallBlindNode.next
        }
      } else {
        fmt.Println("reorderPlayers(): dealer, Sb & Bb all left mid round")
        table.handleOrphanedSeats()
        smallBlindNode = table.SmallBlind
      }
      fmt.Printf("reorderPlayers(): smallblind left mid round, setting curPlayer to %s\n",
                 smallBlindNode.Player.Name)
    }
    smallBlindNode = table.curPlayers.GetPlayerNode(smallBlindNode.Player);
    if smallBlindNode == nil {
    // small-blind folded or is all in so we need to search activePlayers for next actively betting player
      smallBlindNode = table.SmallBlind.next
      for !smallBlindNode.Player.canBet() {
        smallBlindNode = smallBlindNode.next
      }

      smallBlindNode = table.curPlayers.GetPlayerNode(smallBlindNode.Player)

      assert(smallBlindNode != nil, "reorderPlayers(): couldn't find a nonfolded player after Sb")

      fmt.Printf("smallBlind (%s) not active, setting curPlayer to %s\n", table.SmallBlind.Player.Name,
                 smallBlindNode.Player.Name)
    }
    table.curPlayers.node = smallBlindNode // small blind (or next active player)
                                           // is always first better after pre-flop
  }

  table.curPlayer = table.curPlayers.node
}


func (table *Table) handleOrphanedSeats() {
  // TODO: this is the corner case where D, Sb & Bb all leave mid-game. need to
  //       find a way to keep track of dealer pos to rotate properly.
  //
  //       considering making lists doubly linked.
  if table.Dealer == nil && table.SmallBlind == nil && table.BigBlind == nil {
    fmt.Println("handleOrphanedSeats(): D, Sb & Bb all nil, resetting to activePlayers head")
    table.Dealer = table.activePlayers.node
    table.SmallBlind = table.Dealer.next
    table.BigBlind = table.SmallBlind.next
  }
  if table.Dealer == nil && table.SmallBlind == nil && table.BigBlind != nil {
    var newDealerNode *PlayerNode
    for i, n := 0, table.activePlayers.node; i < table.activePlayers.len; i++ {
      if n.next.next.Player.Name == table.BigBlind.Player.Name {
        newDealerNode = n
        break
      }
      n = n.next
    }

    assert(newDealerNode != nil, "handleOrphanedSeats(): newDealerNode == nil")

    fmt.Printf("handleOrphanedSeats(): setting dealer to %s\n", newDealerNode.Player.Name)

    table.Dealer = newDealerNode
    table.SmallBlind = table.Dealer.next
    table.BigBlind = table.SmallBlind.next
  }
  if table.Dealer == nil {
    var newDealerNode *PlayerNode
    for i, n := 0, table.activePlayers.node; i < table.activePlayers.len; i++ {
      if n.next.Player.Name == table.SmallBlind.Player.Name {
        newDealerNode = n
        break
      }
      n = n.next
    }

    assert(newDealerNode != nil, "handleOrphanedSeats(): newDealerNode == nil")

    fmt.Printf("handleOrphanedSeats(): setting dealer to %s\n", newDealerNode.Player.Name)

    table.Dealer = newDealerNode
  } else if table.SmallBlind == nil {
    table.SmallBlind = table.Dealer.next
    fmt.Printf("handleOrphanedSeats(): setting smallblind to %s\n", table.SmallBlind.Player.Name)
  } else if table.BigBlind == nil {
    table.BigBlind = table.SmallBlind.next
    fmt.Printf("handleOrphanedSeats(): setting bigblind to %s\n", table.BigBlind.Player.Name)
  }
}

// rotates the dealer and blinds
func (table *Table) rotatePlayers() {
  if table.State == TABLESTATE_NOTSTARTED || table.activePlayers.len < 2 {
    return
  }

  if table.Dealer == nil || table.SmallBlind == nil || table.BigBlind == nil {
    table.handleOrphanedSeats()
  }

  fmt.Printf("rotatePlayers(): D=%s S=%s B=%s => ",
    table.Dealer.Player.Name,
    table.SmallBlind.Player.Name,
    table.BigBlind.Player.Name)

  Panic := &Panic{}
  Panic.Init()

  defer Panic.ifNoPanic(func() {
    fmt.Printf("D=%s S=%s B=%s\n",
      table.Dealer.Player.Name,
      table.SmallBlind.Player.Name,
      table.BigBlind.Player.Name)

    table.reorderPlayers()
  })

  table.Dealer = table.Dealer.next
  table.SmallBlind = table.Dealer.next
  table.BigBlind = table.SmallBlind.next
}

func (table *Table) setNextPlayerTurn() {
  fmt.Printf("setNextPl called, curP: %s\n", table.curPlayer.Player.Name)
  if table.State == TABLESTATE_NOTSTARTED {
    return
  }

  table.mtx.Lock()
  defer table.mtx.Unlock()

  Panic := &Panic{}
  Panic.Init()

  thisPlayer := table.curPlayer.Player // save in case we need to remove from curPlayers list

  defer Panic.ifNoPanic(func() {
    if table.State == TABLESTATE_DONEBETTING {
      table.better = nil
      table.closeSidePots()
    }

    if thisPlayer.Action.Action == NETDATA_FOLD ||
       thisPlayer.Action.Action == NETDATA_ALLIN {
      nextNode := table.curPlayers.RemovePlayer(thisPlayer)
      if nextNode != nil {
        fmt.Printf("setNextPl(): removing %s, curPs head is %s\n", thisPlayer.Name, table.curPlayers.node.Player.Name)
        table.curPlayer = nextNode
        fmt.Printf("  now %s\n", table.curPlayers.node.Player.Name)
      }
    } else {
      table.curPlayer = table.curPlayer.next
    }

    fmt.Printf(" -> new curP: %v\n", table.curPlayer.Player.Name)
    table.curPlayers.ToPlayerArray()
  })

  if table.curPlayers.len < 2 {
    fmt.Println("table.curPlayers.len < 2")
    if table.allInCount() == 0 {
      fmt.Println("allInCount == 0")
      table.State = TABLESTATE_ROUNDOVER // XXX
    } else {
      table.State = TABLESTATE_DONEBETTING
    }

    return
  } else if table.curPlayer.next == table.curPlayers.node &&
            table.curPlayer.Player.Action.Action != NETDATA_BET {
    fmt.Printf("last player (%s) didn't raise\n", table.curPlayer.Player.Name)
    fmt.Printf(" curPs == %s curP.next == %s\n", table.curPlayers.node.Player.Name, table.curPlayer.next.Player.Name)

    table.State = TABLESTATE_DONEBETTING
  } else {;
    //table.curPlayer = table.curPlayer.next
  }
}

func (table *Table) PlayerAction(player *Player, action Action) error {
  if table.State == TABLESTATE_NOTSTARTED {
    return errors.New("game has not started yet")
  }

  if table.State != TABLESTATE_ROUNDS &&
    table.State != TABLESTATE_PLAYERRAISED &&
    table.State != TABLESTATE_PREFLOP {
    // XXX
    return errors.New("invalid table state: " + table.TableStateToString())
  }

  var blindRequiredBet uint = 0

  isSmallBlindPreFlop := false

  if table.CommState == TABLESTATE_PREFLOP { // XXX mixed states...
    if table.SmallBlind != nil && player.Name == table.SmallBlind.Player.Name {
      isSmallBlindPreFlop = true
      blindRequiredBet = table.Ante / 2
    } else if table.BigBlind!= nil && player.Name == table.BigBlind.Player.Name {
      blindRequiredBet = table.Ante
    }
  }

  handleSidePots := func(isFirstSidePot bool) {
    if isFirstSidePot {
      sidePot := &SidePot{
        Bet: table.Bet - player.Action.Amount,
      }
      sidePot.Init(nil, nil)

      // get players who already called the last bet,
      // sub the delta of the last bet and this players
      // chipcount in mainpot, then add them to the sidepot.
      for _, p := range table.getNonFoldedPlayers() {
        if p.Name == player.Name {
          break // this player pos is same as table pos
        }

        table.MainPot.Pot -= sidePot.Bet

        printer.Printf("is1stSP: <%s> sub %d from mainpot, add same amt to sidePot\n", p.Name, sidePot.Bet)

        sidePot.Pot += sidePot.Bet
        sidePot.Players[p.Name] = p
      }

      table.SidePot = append(table.SidePot, sidePot)

      table.MainPot.Bet = player.Action.Amount
      table.MainPot.Pot += table.MainPot.Bet

      return
    }

    if table.SidePot[len(table.SidePot)-1].IsClosed {
      sidePot := &SidePot{
        Bet: player.Action.Amount,
        Pot: player.Action.Amount,
      }
      sidePot.Init(nil, player)

      table.SidePot = append(table.SidePot, sidePot)
      printer.Printf("isClosed: added new sidepot bet: %d pot: %d p: %d\n",
        sidePot.Bet, sidePot.Pot, sidePot.Players[player.Name].Name)
    } else {
      latestSidePotIdx := len(table.SidePot) - 1
      latestSidePot := table.SidePot[latestSidePotIdx]

      /*if len(latestSidePot.Players) == len(players) {
        // finished adding all players to the open sidepot
        latestSidePot.IsClosed = true
      }*/

      if player.Action.Action == NETDATA_ALLIN &&
        player.Action.Amount < latestSidePot.Bet {
        // make a new sidepot if player is allin with amt < bet
        sidePot := &SidePot{
          Bet: player.Action.Amount,
          Pot: player.Action.Amount * uint(len(latestSidePot.Players)),
        }

        if p := latestSidePot.Players[player.Name]; p == nil {
          // current players wasn't part of previous sidepot
          sidePot.Pot += sidePot.Bet
        }

        sidePot.Init(latestSidePot.Players, player)

        table.SidePot = append(table.SidePot, sidePot)

        // swap to make sure betting sidepot is the tail
        table.SidePot[latestSidePotIdx+1], table.SidePot[latestSidePotIdx] = table.SidePot[latestSidePotIdx],
          table.SidePot[latestSidePotIdx+1]

        fmt.Printf("!isClosed: p.amt (%d) < bet (%d): added new sidepot bet: %v pot: %v p: %v\n",
          player.Action.Amount, latestSidePot.Bet, sidePot.Bet, sidePot.Pot, sidePot.Players[player.Name].Name)
      } else {
        if !table.MainPot.IsClosed {
          assert(player.ChipCount >= table.MainPot.Bet, printer.Sprintf("TMP: %v cc: %v cant match mainpot bet %v",
                                                                        player.Name, player.ChipCount, table.MainPot.Bet))

          table.MainPot.Pot += table.MainPot.Bet
          player.Action.Amount -= table.MainPot.Bet
          player.ChipCount -= table.MainPot.Bet
        }

        if latestSidePot.Players[player.Name] == nil {
          latestSidePot.Players[player.Name] = player
        }

        // add current player to open sidepots. this happens when multiple
        // players go all-in.
        for i, sidePot := range table.SidePot[:len(table.SidePot)-1] {
          if !sidePot.IsClosed {
            assert(player.ChipCount >= sidePot.Bet, "TMP: player cant match a sidePot bet")

            sidePot.Pot += sidePot.Bet

            sidePot.Players[player.Name] = player

            printer.Printf("adding %s to open sidePot (#%d), prev: p.amt %d p.cc %d", i,
              player.Action.Amount, player.ChipCount)

            player.Action.Amount -= sidePot.Bet // all-ins get subtracted from this bet
            player.ChipCount -= sidePot.Bet

            printer.Printf(" new: p.amt %d p.cc %d\n", player.Action.Amount, player.ChipCount)
          }
        }

        switch player.Action.Action {
        case NETDATA_BET:
          lspb := latestSidePot.Bet
          if table.State == TABLESTATE_PLAYERRAISED &&
            player.Action.Amount > latestSidePot.Bet {
            fmt.Printf("!isClosed: %s re-raised\n", player.Name)
            latestSidePot.Pot += player.Action.Amount - latestSidePot.Bet
            latestSidePot.Bet = player.Action.Amount
          } else {
            fmt.Printf("!isClosed: %s made new bet\n", player.Name)
            latestSidePot.Bet = player.Action.Amount
            latestSidePot.Pot += latestSidePot.Bet
          }
          if latestSidePot.Bet != lspb {
            printer.Printf("!isClosed: %s changed sidepot bet from %d to %d\n", player.Name,
              lspb, player.Action.Amount)
          }
        case NETDATA_CALL:
          fmt.Printf("!isClosed: %s called\n", player.Name)
          latestSidePot.Pot += latestSidePot.Bet
        }

      }
    }
  }

  // TODO: check back..
  if action.Action == NETDATA_ALLIN && table.bettingIsImpossible() {
    fmt.Printf("PlayerAction(): last player (%s) went all-in, changing action to call\n", player.Name)
    action.Action = NETDATA_CALL
  }

  switch action.Action {
  case NETDATA_ALLIN:
    player.Action.Action = NETDATA_ALLIN

    // we need to add the blind's chips back, otherwise it would get added to current bet
    player.Action.Amount -= blindRequiredBet
    player.ChipCount += blindRequiredBet

    chipLeaderCount, secondChipLeaderCount := table.getChipLeaders()

    // NOTE: A chipleader can only bet what at least one other player can match.
    if player.ChipCount == chipLeaderCount {
      player.Action.Amount = secondChipLeaderCount
    } else {
      player.Action.Amount = player.ChipCount
    }

    if player.Action.Amount > table.Bet {
      table.Bet = player.Action.Amount
      table.State = TABLESTATE_PLAYERRAISED
      table.better = player
      fmt.Printf("setting curPlayers head to %s\n", table.curPlayer.Player.Name)
      table.curPlayers.node = table.curPlayer // NOTE: the new better always becomes the head of the table
    }

    if len(table.SidePot) > 0 { // then some active player can't match bet
      handleSidePots(false)
    } else if table.State == TABLESTATE_PLAYERRAISED &&
      player.Action.Amount < table.Bet { // first sidepot
      handleSidePots(true)
    } else { // this player can match bet
      table.MainPot.Pot += player.Action.Amount
    }

    player.ChipCount -= player.Action.Amount

    //table.curPlayers.RemovePlayer(player)
  case NETDATA_BET:
    if action.Amount < table.Ante {
      return errors.New(printer.Sprintf("bet must be greater than the ante (%d chips)", table.Ante))
    } else if action.Amount <= table.Bet {

      return errors.New(printer.Sprintf("bet must be greater than the current bet (%d chips)", table.Bet))
    } else if action.Amount+blindRequiredBet > player.ChipCount {
      return errors.New("not enough chips")
    }

    // we need to add the blind's chips back, otherwise it would get added to current bet
    player.Action.Amount -= blindRequiredBet
    player.ChipCount += blindRequiredBet

    chipLeaderCount, secondChipLeaderCount := table.getChipLeaders()

    // NOTE: A chipleader can only bet what at least one other player can match.
    if player.ChipCount == chipLeaderCount {
      player.Action.Amount = minUInt(action.Amount, secondChipLeaderCount)
    } else {
      player.Action.Amount = action.Amount
    }

    if action.Amount == player.ChipCount {
      player.Action.Action = NETDATA_ALLIN
    } else {
      player.Action.Action = NETDATA_BET
    }

    fmt.Printf("%s bet b4 splogic: %v\n", player.Name, player.Action.Amount)

    if len(table.SidePot) > 0 {
      handleSidePots(false)
    } else {
      table.MainPot.Pot += player.Action.Amount
    }

    fmt.Printf("after splogic %v\n", player.Action.Amount)

    player.ChipCount -= player.Action.Amount

    table.Bet = player.Action.Amount

    fmt.Printf("setting curPlayers head to %s\n", table.curPlayer.Player.Name)
    table.curPlayers.node = table.curPlayer
    table.better = player
    table.State = TABLESTATE_PLAYERRAISED
  case NETDATA_CALL:
    if table.State != TABLESTATE_PLAYERRAISED && !isSmallBlindPreFlop {
      return errors.New("nothing to call")
    }

    if (table.SmallBlind != nil && player.Name == table.SmallBlind.Player.Name) ||
       (table.BigBlind != nil && player.Name == table.BigBlind.Player.Name) {
      fmt.Printf("PlayerAction(): CALL: %s action.amt: %v\n", player.Name, player.Action.Amount)
    }

    // XXX we need to add the blind's chips back, otherwise it would get added to current bet
    // NOTE: Amount is always >= blindRequiredBet
    player.Action.Amount -= blindRequiredBet
    player.ChipCount += blindRequiredBet

    // delta of bet & curPlayer's last bet
    betDiff := table.Bet - player.Action.Amount

    if betDiff >= player.ChipCount {
      player.Action.Action = NETDATA_ALLIN
      player.Action.Amount = player.ChipCount

      if len(table.SidePot) > 0 {
        handleSidePots(false)
      } else if betDiff > player.ChipCount {
        handleSidePots(true)
      } else {
        table.MainPot.Pot += player.ChipCount
      }

      player.ChipCount = 0
    } else {
      player.Action.Action = NETDATA_CALL

      player.Action.Amount = table.Bet

      if len(table.SidePot) > 0 {
        handleSidePots(false)
      } else {
        player.ChipCount -= betDiff
        table.MainPot.Pot += betDiff
      }
    }
  case NETDATA_CHECK:
    if table.State == TABLESTATE_PLAYERRAISED {
      return errors.New(printer.Sprintf("must call the raise (%d chips)", table.Bet))
    }

    if isSmallBlindPreFlop {
      return errors.New(printer.Sprintf("must call the big blind (+%d chips)", blindRequiredBet))
    }

    if player.ChipCount == 0 { // big blind had a chipcount <= ante
      player.Action.Action = NETDATA_ALLIN
    } else {
      player.Action.Action = NETDATA_CHECK
    }
    // for bigblind preflop
    table.MainPot.Pot += minUInt(player.ChipCount, blindRequiredBet)
  case NETDATA_FOLD:
    player.Action.Action = NETDATA_FOLD

    table.MainPot.Pot += blindRequiredBet

    //table.curPlayers.RemovePlayer(player)
  default:
    return errors.New("BUG: invalid player action: " + strconv.Itoa(action.Action))
  }

  table.setNextPlayerTurn()

  return nil
}

func (table *Table) Deal() {
  for _, player := range table.curPlayers.ToPlayerArray() {
    player.Hole.Cards = append(player.Hole.Cards, table.deck.Pop())
    player.Hole.Cards = append(player.Hole.Cards, table.deck.Pop())

    player.Hole.FillHoleInfo()
  }

  table.State = TABLESTATE_PREFLOP
}

func (table *Table) AddToCommunity(card *Card) {
  table.Community = append(table.Community, card)
  table._comsorted = append(table._comsorted, card)
}

// print name of current community cards to stdout
func (table *Table) PrintCommunity() {
  for _, card := range table.Community {
    fmt.Printf(" [%s]", card.Name)
  }
  fmt.Println()
}

func (table *Table) PrintSortedCommunity() {
  fmt.Printf("sorted: ")
  for _, card := range table._comsorted {
    fmt.Printf(" [%s]", card.Name)
  }
  fmt.Println()
}

// sort community cards by number
func (table *Table) SortCommunity() {
  cardsSort(&table._comsorted)
}

func (table *Table) nextCommunityAction() {
  switch table.CommState {
  case TABLESTATE_PREFLOP:
    table.DoFlop()

    table.CommState = TABLESTATE_FLOP
    if !table.bettingIsImpossible() { // else all players went all in preflop
                                      // and we are in the all-in loop
      table.reorderPlayers()
    }
  case TABLESTATE_FLOP:
    table.DoTurn()

    table.CommState = TABLESTATE_TURN
  case TABLESTATE_TURN:
    table.DoRiver()

    table.CommState = TABLESTATE_RIVER
  case TABLESTATE_RIVER:
    table.State = TABLESTATE_ROUNDOVER // XXX shouldn't mix these states

    return
  default:
    panic("BUG: nextCommunityAction(): invalid community state")
  }

  table.State = TABLESTATE_ROUNDS
}

func (table *Table) nextTableAction() {
  switch table.State {
  case TABLESTATE_NOTSTARTED:
    if table.Dealer == nil || table.SmallBlind == nil || table.BigBlind == nil {
      table.handleOrphanedSeats()
    }

    table.Bet = table.Ante
    table.SmallBlind.Player.Action.Amount = minUInt(table.Ante/2, table.SmallBlind.Player.ChipCount)
    table.BigBlind.Player.Action.Amount = minUInt(table.Ante, table.BigBlind.Player.ChipCount)

    table.SmallBlind.Player.ChipCount -= table.SmallBlind.Player.Action.Amount
    table.BigBlind.Player.ChipCount -= table.BigBlind.Player.Action.Amount

    table.Deal()

    table.CommState = TABLESTATE_PREFLOP

    table.reorderPlayers() // NOTE: need to call this to properly set curPlayer
  case TABLESTATE_NEWROUND:
    table.rotatePlayers()

    table.SmallBlind.Player.Action.Amount = minUInt(table.Ante/2,
                                                    table.SmallBlind.Player.ChipCount)
    table.BigBlind.Player.Action.Amount = minUInt(table.Ante,
                                                  table.BigBlind.Player.ChipCount)

    table.SmallBlind.Player.ChipCount -= table.SmallBlind.Player.Action.Amount
    table.BigBlind.Player.ChipCount -= table.BigBlind.Player.Action.Amount

    if table.SmallBlind.Player.ChipCount == 0 {
      table.SmallBlind.Player.Action.Action = NETDATA_ALLIN
    }
    if table.BigBlind.Player.ChipCount == 0 {
      table.BigBlind.Player.Action.Action = NETDATA_ALLIN
    }

    table.Deal()

    table.CommState = TABLESTATE_PREFLOP
  case TABLESTATE_GAMEOVER:
    fmt.Printf("nextTableAction(): game over!\n")

  default:
    fmt.Printf("nextTableAction(): BUG: called with improper state (" +
      table.TableStateToString() + ")")
  }
}

func (table *Table) DoFlop() {
  for i := 0; i < 3; i++ {
    table.AddToCommunity(table.deck.Pop())
  }
  table.PrintCommunity()
  table.SortCommunity()

  table.State = TABLESTATE_ROUNDS
}

func (table *Table) DoTurn() {
  table.AddToCommunity(table.deck.Pop())
  table.PrintCommunity()
  table.SortCommunity()
}

func (table *Table) DoRiver() {
  table.AddToCommunity(table.deck.Pop())
  table.PrintCommunity()
  table.SortCommunity()
}

// we need to define this function at this scope because
// it is recursive.
func checkTies(players []*Player, cardidx int) []*Player {
  if len(players) == 1 || cardidx == -1 {
    // one player left or remaining players tied fully
    return players
  }

  best := []*Player{players[0]}

  for _, player := range players[1:] {
    if player.Hand.Cards[cardidx].NumValue == best[0].Hand.Cards[cardidx].NumValue {
      best = append(best, player)
    } else if player.Hand.Cards[cardidx].NumValue > best[0].Hand.Cards[cardidx].NumValue {
      best = []*Player{player}
    }
  }

  return checkTies(best, cardidx-1)
}

func (table *Table) getNonFoldedPlayers() []*Player {
  players := make([]*Player, 0)

  for _, player := range table.getActiveSeats() {
    if player.Action.Action != NETDATA_FOLD {
      players = append(players, player)
    }
  }

  assert(len(players) != 0, "getNonFoldedPlayers(): BUG: len(players) == 0")

  return players
}

func (table *Table) newRound() {
  table.deck.Shuffle()

  table.addNewPlayers()

  for _, player := range table.activePlayers.ToPlayerArray() {
    player.NewCards()

    player.Action.Amount = 0
    player.Action.Action = NETDATA_FIRSTACTION // NOTE: set twice w/ new player
  }

  table.newCommunity()

  table.roundCount++

  if table.roundCount%10 == 0 {
    table.Ante *= 2 // TODO increase with time interval instead
  }

  table.handleOrphanedSeats()

  table.curPlayers = table.activePlayers.Clone("curPlayers")
  table.better = nil
  table.Bet = table.Ante // min bet is big blind bet
  table.MainPot.Pot = 0  // XXX
  table.SidePot = nil
  table.State = TABLESTATE_NEWROUND
}

func (table *Table) finishRound() {
  // special case for when everyone except a folded player
  // leaves the table
  if table.activePlayers.len == 1 &&
     table.activePlayers.node.Player.Action.Action == NETDATA_FOLD {
    fmt.Printf("finishRound(): only one folded player (%s) left at table. abandoning all pots\n",
               table.activePlayers.node.Player.Name)

    table.State = TABLESTATE_GAMEOVER
    table.Winners = []*Player{table.activePlayers.node.Player}

    return
  }

  players := table.getNonFoldedPlayers()

  printer.Printf("mainpot: %d\n", table.MainPot.Pot)
  for i, sidePot := range table.SidePot {
    printer.Printf("sp %v - bet: %v pot: %v closed: %v\n", i, sidePot.Bet, sidePot.Pot, sidePot.IsClosed)
    printer.Printf(" players: ")
    for p := range sidePot.Players {
      printer.Printf("%s, ", p)
    }
    fmt.Println()
  }

  if len(players) == 1 { // win by folds
    player := players[0]

    player.ChipCount += table.MainPot.Pot

    assert(len(table.SidePot) == 0,
      printer.Sprintf("BUG: finishRound(): %s won by folds but there are sidepots", player.Name))

    table.State = TABLESTATE_ROUNDOVER

    table.Winners = players

    return
  }

  table.State = TABLESTATE_SHOWHANDS

  bestPlayers := table.BestHand(players, nil)

  // TODO: redundant code
  if len(bestPlayers) == 1 {
    bestPlayers[0].ChipCount += table.MainPot.Pot
  } else {
    splitChips := table.MainPot.Pot / uint(len(bestPlayers))

    printer.Printf("mainpot: split chips: %v\n", splitChips)

    for _, player := range bestPlayers {
      player.ChipCount += splitChips
    }

    table.State = TABLESTATE_SPLITPOT
  }

  table.Winners = bestPlayers

  for i, sidePot := range table.SidePot {
    // remove players that folded from sidePots
    // XXX: probably not the best place to do this.
    for _, player := range sidePot.Players {
      if player.Action.Action == NETDATA_FOLD {
        fmt.Printf("removing %s from sidePot #%d\n", player.Name, i)
        delete(sidePot.Players, player.Name)
      }
    }

    if len(sidePot.Players) == 0 {
      fmt.Printf("sidePot #%d has no players attached. skipping..\n", i)
      continue
    }

    if len(sidePot.Players) == 1 { // win by folds
      var player *Player
      // XXX
      for _, p := range sidePot.Players {
        player = p
      }

      fmt.Printf("%s won sidePot #%d by folds\n", player.Name, i)

      player.ChipCount += sidePot.Pot

      table.Winners = append(table.Winners, player)
    } else {
      sidePotPlayersArr := make([]*Player, 0, len(sidePot.Players))
      for _, player := range sidePot.Players { // TODO: make a mapval2slice util func
        sidePotPlayersArr = append(sidePotPlayersArr, player)
      }
      bestPlayers := table.BestHand(sidePotPlayersArr, sidePot)

      if len(bestPlayers) == 1 {
        fmt.Printf("%s won sidePot #%d\n", bestPlayers[0].Name, i)
        bestPlayers[0].ChipCount += sidePot.Pot
      } else {
        splitChips := sidePot.Pot / uint(len(bestPlayers))

        printer.Printf("sidepot #%d: split chips: %v\n", i, splitChips)

        for _, player := range bestPlayers {
          player.ChipCount += splitChips
        }

        //table.State = TABLESTATE_SPLITPOT
      }

      table.Winners = append(table.Winners, bestPlayers...)
    }
  }
}

func (table *Table) BestHand(players []*Player, sidePot *SidePot) []*Player {
  var winInfo *string
  if sidePot == nil {
    winInfo = &table.WinInfo
    //table.WinInfo = table.CommunityToString() + "\n\n"
    *winInfo = table.CommunityToString() + "\n\n"
  } else {
    winInfo = &sidePot.WinInfo
  }

  if sidePot == nil {
    for _, player := range players {
      assembleBestHand(false, table, player)

      *winInfo += fmt.Sprintf("%s [%4s][%4s] => %-15s (rank %d)\n",
        player.Name,
        player.Hole.Cards[0].Name, player.Hole.Cards[1].Name,
        player.Hand.RankName(), player.Hand.Rank)

      fmt.Printf("%s [%4s][%4s] => %-15s (rank %d)\n", player.Name,
        player.Hole.Cards[0].Name, player.Hole.Cards[1].Name,
        player.Hand.RankName(), player.Hand.Rank)
    }
  }

  bestPlayers := []*Player{players[0]}

  for _, player := range players[1:] {
    if player.Hand.Rank == bestPlayers[0].Hand.Rank {
      bestPlayers = append(bestPlayers, player)
    } else if player.Hand.Rank > bestPlayers[0].Hand.Rank {
      bestPlayers = []*Player{player}
    }
  }

  tiedPlayers := checkTies(bestPlayers, 4)

  if len(tiedPlayers) > 1 {
    // split pot
    fmt.Printf("split pot between ")
    *winInfo += "split pot between "
    for _, player := range tiedPlayers {
      fmt.Printf("%s ", player.Name)
      *winInfo += player.Name + " "
    }
    fmt.Printf("\r\n")

    *winInfo += "\nwinning hand => " + tiedPlayers[0].Hand.RankName() + "\n"
    fmt.Printf("winning hand => %s\n", tiedPlayers[0].Hand.RankName())
  } else {
    *winInfo += "\n" + tiedPlayers[0].Name + "  wins with " + tiedPlayers[0].Hand.RankName() + "\n"
    fmt.Printf("\n%s wins with %s\n", tiedPlayers[0].Name, tiedPlayers[0].Hand.RankName())
  }

  // print the best hand
  for _, card := range tiedPlayers[0].Hand.Cards {
    fmt.Printf("[%4s]", card.Name)
    *winInfo += fmt.Sprintf("[%4s]", card.Name)
  }
  fmt.Println()

  return tiedPlayers
}

// hand matching logic unoptimized
func assembleBestHand(preshow bool, table *Table, player *Player) {
  var restoreHand Hand
  if player.Hand != nil {
    restoreHand = *player.Hand
  } else {
    restoreHand = Hand{}
  }
  defer func() {
    // XXX TODO very temporary!
    if preshow {
      if player.Hand != nil {
        player.PreHand = *player.Hand
      } else {
        player.PreHand = Hand{}
      }
      player.Hand = &restoreHand
    }
  }()

  if table.State == TABLESTATE_PREFLOP ||
    len(player.Hole.Cards) != 2 ||
    len(table.Community) < 3 {
    return
  }

  cards := append(table.Community, player.Hole.Cards...)
  cardsSort(&cards)
  bestCard := len(cards)

  // get all the pairs/trips/quads into one slice
  // NOTE: ascending order
  //
  // this struct keeps a slice of the match type indexes
  // in ascending order
  var matchHands struct {
    quads []uint
    trips []uint
    pairs []uint
  }

  matching_cards := false

  // NOTE: double loop not optimal, readability trade-off okay for given slice size
  for i := 0; i < len(cards)-1; {
    match_num := 1

    for _, adj_card := range cards[i+1:] {
      if cards[i].NumValue == adj_card.NumValue {
        match_num++
      } else {
        break
      }
    }

    if match_num > 1 {
      if !matching_cards {
        matching_cards = true
      }

      var matchmemb *[]uint
      switch match_num {
      case 4:
        matchmemb = &matchHands.quads
      case 3:
        matchmemb = &matchHands.trips
      case 2:
        matchmemb = &matchHands.pairs
      }
      *matchmemb = append(*matchmemb, uint(i))

      i += match_num
    } else {
      i++
    }
  }

  // used for tie breakers
  // this func assumes the card slice is sorted and ret will be <= 5
  top_cards := func(cards Cards, num int, except []int) Cards {
    ret := make(Cards, 0, 5)

    assert(len(cards) <= 7, "too many cards in top_cards()")

    for i := len(cards) - 1; i >= 0; i-- {
      skip := false
      if len(ret) == num {
        return ret
      }

      for _, except_numvalue := range except {
        if cards[i].NumValue == except_numvalue {
          skip = true
          break
        }
      }

      if !skip {
        // insert at beginning of slice
        ret = append(Cards{cards[i]}, ret...)
      }
    }

    return ret
  }

  // flush search function //
  gotFlush := func(cards Cards, player *Player, addToCards bool) (bool, int) {
    type _suitstruct struct {
      cnt   uint
      cards Cards
    }
    suits := make(map[int]*_suitstruct)

    for _, card := range cards {
      suits[card.Suit] = &_suitstruct{cards: Cards{}}
    }

    // count each suit
    for _, card := range cards {
      suits[card.Suit].cnt++
      suits[card.Suit].cards = append(suits[card.Suit].cards, card)
    }

    // search for flush
    for suit, suit_struct := range suits {
      if suit_struct.cnt >= 5 { // NOTE: it's only possible to get one flush
        player.Hand.Rank = R_FLUSH

        if addToCards {
          player.Hand.Cards = append(player.Hand.Cards,
            suit_struct.cards[len(suit_struct.cards)-5:len(suit_struct.cards)]...)
        }
        return true, suit
      }
    }

    return false, 0
  }

  // straight flush/straight search function //
  gotStraight := func(cards *Cards, player *Player, high int, acelow bool) bool {
    straightFlush := true

    if acelow {
      // check ace to 5
      acesuit := (*cards)[len(*cards)-1].Suit

      if (*cards)[0].NumValue != C_TWO {
        return false // can't be A to 5
      }

      for i := 1; i <= high; i++ {
        if (*cards)[i].Suit != acesuit {
          straightFlush = false
        }

        if (*cards)[i].NumValue != (*cards)[i-1].NumValue+1 {
          return false
        }
      }
    } else {
      low := high - 4
      for i := high; i > low; i-- {
        //fmt.Printf("h %d L %d ci %d ci-1 %d\n", high, low, i, i-1)
        if (*cards)[i].Suit != (*cards)[i-1].Suit { // XXX had [i-1].Suit+1 for some reason
          straightFlush = false
        }

        if (*cards)[i].NumValue != (*cards)[i-1].NumValue+1 {
          return false
        }
      }
    }

    if straightFlush {
      if (*cards)[high].NumValue == C_ACE {
        player.Hand.Rank = R_ROYALFLUSH
      } else {
        player.Hand.Rank = R_STRAIGHTFLUSH
      }
    } else {
      player.Hand.Rank = R_STRAIGHT
    }

    if acelow {
      player.Hand.Cards = append(Cards{(*cards)[len(*cards)-1]}, (*cards)[:4]...)
    } else {
      player.Hand.Cards = append(player.Hand.Cards, (*cards)[high-4:high+1]...)
    }
    assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

    return true
  }

  if !matching_cards {
    // best possible hands with no matches in order:
    // royal flush, straight flush, flush, straight or high card.
    // XXX: make better
    // we check for best straight first to reduce cycles
    //for i := 1; i < 4; i++ {
    isStraight := false

    for i := 1; i < len(cards)-3; i++ {
      if gotStraight(&cards, player, bestCard-i, false) {
        isStraight = true
        break
      }
    }

    if player.Hand.Rank == R_ROYALFLUSH ||
      player.Hand.Rank == R_STRAIGHTFLUSH {
      return
    }

    if isFlush, _ := gotFlush(cards, player, true); isFlush {
      return
    }

    // check for A to 5
    if !isStraight && cards[len(cards)-1].NumValue == C_ACE {
      gotStraight(&cards, player, 3, true)
    }

    if player.Hand.Rank == R_STRAIGHT {
      return
    }

    // muck
    player.Hand.Rank = R_HIGHCARD
    player.Hand.Cards = append(player.Hand.Cards, cards[bestCard-1],
      cards[bestCard-2], cards[bestCard-3],
      cards[bestCard-4], cards[bestCard-5])

    assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

    return
  }

  // quads search //
  if matchHands.quads != nil {
    quadsIdx := int(matchHands.quads[0]) // 0 because it's impossible to
    // get quads twice
    kicker := &Card{}
    for i := bestCard - 1; i >= 0; i-- { // kicker search
      if cards[i].NumValue > cards[quadsIdx].NumValue {
        kicker = cards[i]
        break
      }
    }

    assert(kicker != nil, "quads: kicker == nil")

    player.Hand.Rank = R_QUADS
    player.Hand.Cards = append(Cards{kicker}, cards[quadsIdx:quadsIdx+4]...)

    return
  }

  // fullhouse search //
  //
  // NOTE: we check for a fullhouse before a straight flush because it's
  // impossible to have both at the same time and searching for the fullhouse
  // first saves some cycles+space
  if matchHands.trips != nil && matchHands.pairs != nil {
    player.Hand.Rank = R_FULLHOUSE

    pairIdx := int(matchHands.pairs[len(matchHands.pairs)-1])
    tripsIdx := int(matchHands.trips[len(matchHands.trips)-1])

    player.Hand.Cards = append(player.Hand.Cards, cards[pairIdx:pairIdx+2]...)
    player.Hand.Cards = append(player.Hand.Cards, cards[tripsIdx:tripsIdx+3]...)

    assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

    return
  }

  // flush search //
  //
  // NOTE: we search for the flush here to ease the straight flush logic
  haveFlush, suit := gotFlush(cards, player, false)

  // remove duplicate card (by number) for easy straight search
  uniqueCards := Cards{}

  if haveFlush {
    // check for possible RF/straight flush suit
    cardmap := make(map[int]int) // key == num, val == suit

    for _, card := range cards {
      mappedsuit, found := cardmap[card.NumValue]

      if found && mappedsuit != suit && card.Suit == suit {
        cardmap[card.NumValue] = card.Suit
        assert(uniqueCards[len(uniqueCards)-1].NumValue == card.NumValue, "uniqueCards problem")
        uniqueCards[len(uniqueCards)-1] = card // should _always_ be last card
      } else if !found {
        cardmap[card.NumValue] = card.Suit
        uniqueCards = append(uniqueCards, card)
      }
    }

    assert((len(uniqueCards) <= 7 && len(uniqueCards) >= 3),
      fmt.Sprintf("impossible number of unique cards (%v)", len(uniqueCards)))
  } else {
    cardmap := make(map[int]bool)

    for _, card := range cards {
      if _, val := cardmap[card.NumValue]; !val {
        cardmap[card.NumValue] = true
        uniqueCards = append(uniqueCards, card)
      }
    }

    assert((len(uniqueCards) <= 7 && len(uniqueCards) >= 1),
      "impossible number of unique cards")
  }

  // RF, SF and straight search //
  if len(uniqueCards) >= 5 {
    uniqueBestCard := len(uniqueCards)
    iter := uniqueBestCard - 4
    isStraight := false
    //fmt.Printf("iter %v len(uc) %d\n)", iter, len(uniqueCards))

    for i := 1; i <= iter; i++ {
      if gotStraight(&uniqueCards, player, uniqueBestCard-i, false) {
        assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
        isStraight = true
        break
      }
    }

    if player.Hand.Rank == R_ROYALFLUSH ||
      player.Hand.Rank == R_STRAIGHTFLUSH {
      return
    }

    if !isStraight && uniqueCards[uniqueBestCard-1].NumValue == C_ACE &&
      gotStraight(&uniqueCards, player, 4, true) {
      assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
    }
  }

  if haveFlush {
    gotFlush(cards, player, true)

    assert(player.Hand.Rank == R_FLUSH, "player should have a flush")

    return
  }

  if player.Hand.Rank == R_STRAIGHT {
    return
  }

  // trips search
  if matchHands.trips != nil {
    firstCard := int(matchHands.trips[len(matchHands.trips)-1])

    tripslice := make(Cards, 0, 3)
    tripslice = append(tripslice, cards[firstCard:firstCard+3]...)

    kickers := top_cards(cards, 2, []int{cards[firstCard].NumValue})
    // order => [kickers][trips]
    kickers = append(kickers, tripslice...)

    player.Hand.Rank = R_TRIPS
    player.Hand.Cards = kickers

    return
  }

  // two pair & pair search
  if matchHands.pairs != nil {
    if len(matchHands.pairs) > 1 {
      player.Hand.Rank = R_2PAIR
      highPairIdx := int(matchHands.pairs[len(matchHands.pairs)-1])
      lowPairIdx := int(matchHands.pairs[len(matchHands.pairs)-2])

      player.Hand.Cards = append(player.Hand.Cards, cards[lowPairIdx:lowPairIdx+2]...)
      player.Hand.Cards = append(player.Hand.Cards, cards[highPairIdx:highPairIdx+2]...)

      kicker := top_cards(cards, 1, []int{cards[highPairIdx].NumValue,
        cards[lowPairIdx].NumValue})
      player.Hand.Cards = append(kicker, player.Hand.Cards...)
    } else {
      player.Hand.Rank = R_PAIR
      pairidx := matchHands.pairs[0]
      kickers := top_cards(cards, 3, []int{cards[pairidx].NumValue})
      player.Hand.Cards = append(kickers, cards[pairidx:pairidx+2]...)
    }

    return
  }

  // muck
  player.Hand.Rank = R_HIGHCARD
  player.Hand.Cards = append(player.Hand.Cards, cards[bestCard-1],
    cards[bestCard-2], cards[bestCard-3],
    cards[bestCard-4], cards[bestCard-5])

  return
}

func cardsSort(cards *Cards) error {
  sort.Slice((*cards), func(i, j int) bool {
    return (*cards)[i].NumValue < (*cards)[j].NumValue
  })

  return nil
}

func cardNumToString(card *Card) error {
  cardNumStringMap := map[int]string{
    C_TWO:   "2",
    C_THREE: "3",
    C_FOUR:  "4",
    C_FIVE:  "5",
    C_SIX:   "6",
    C_SEVEN: "7",
    C_EIGHT: "8",
    C_NINE:  "9",
    C_TEN:   "10",
    C_JACK:  "J",
    C_QUEEN: "Q",
    C_KING:  "K",
    C_ACE:   "A",
  }

  name := cardNumStringMap[card.NumValue]
  if name == "" {
    fmt.Println("cardNumToString(): BUG")
    fmt.Printf("c: %s %d %d\n", card.Name, card.NumValue, card.Suit)
    return errors.New("cardNumToString")
  }

  cardSuitStringMap := map[int][]string{
    S_CLUB:    {"♣", "clubs"},
    S_DIAMOND: {"♦", "diamonds"},
    S_HEART:   {"♥", "hearts"},
    S_SPADE:   {"♠", "spades"},
  }

  suitName := cardSuitStringMap[card.Suit]
  if suitName == nil {
    // TODO: fix redundancy.
    fmt.Println("cardNumToString(): BUG")
    fmt.Printf("c: %s %d %d\n", card.Name, card.NumValue, card.Suit)
    return errors.New("cardNumToString")
  }

  suit, suit_full := suitName[0], suitName[1]

  card.Name = name + " " + suit
  card.FullName = name + " of " + suit_full

  return nil
}

func assert(cond bool, msg string) {
  if !cond {
    panic(msg)
  }
}

func absUint(x, y uint) uint {
  if x > y {
    return x - y
  }

  return y - x
}

func minUInt(x, y uint) uint {
  if x < y {
    return x
  }

  return y
}

// used to avoid execution of defers after a panic()
type Panic struct {
  panicked  bool
  panic     func(string)
  ifNoPanic func(func())
}

func (p *Panic) Init() {
  p.panicked = false

  p.panic = func(msg string) {
    p.panicked = true
    panic(msg)
  }

  p.ifNoPanic = func(deferredFunc func()) {
    if !p.panicked {
      deferredFunc()
    }
  }
}

func panicRetToError(err interface{}) error {
  var typedErr error

  switch errType := err.(type) {
  case string:
    typedErr = errors.New(errType)
  case error:
    typedErr = errType
  default:
    typedErr = errors.New("unknown panic")
  }

  return typedErr
}

func randSeed() {
  var b [8]byte

  _, err := crypto_rand.Read(b[:])
  if err != nil {
    panic("problem with crypto/rand")
  }

  math_rand.Seed(int64(binary.LittleEndian.Uint64(b[:])))
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func randString(n int) string {
  b := make([]rune, n)

  for i := range b {
    b[i] = letters[math_rand.Intn(len(letters))]
  }

  randSeed() // re-seed just in case

  return string(b)
}

// requests/responses sent between client and server
const (
  NETDATA_CLOSE = iota
  NETDATA_NEWCONN

  NETDATA_YOURPLAYER
  NETDATA_NEWPLAYER
  NETDATA_CURPLAYERS
  NETDATA_UPDATEPLAYER
  NETDATA_UPDATETABLE
  NETDATA_PLAYERLEFT
  NETDATA_CLIENTEXITED
  NETDATA_RESET

  NETDATA_SERVERCLOSED

  NETDATA_MAKEADMIN
  NETDATA_STARTGAME

  NETDATA_CHATMSG

  NETDATA_PLAYERACTION
  NETDATA_PLAYERTURN
  NETDATA_ALLIN
  NETDATA_BET
  NETDATA_CALL
  NETDATA_CHECK
  NETDATA_RAISE
  NETDATA_FOLD

  NETDATA_CURHAND
  NETDATA_SHOWHAND

  NETDATA_FIRSTACTION
  NETDATA_MIDROUNDADDITION
  NETDATA_ELIMINATED
  NETDATA_VACANTSEAT

  NETDATA_DEAL
  NETDATA_FLOP
  NETDATA_TURN
  NETDATA_RIVER
  NETDATA_BESTHAND
  NETDATA_ROUNDOVER

  NETDATA_SERVERMSG
  NETDATA_BADREQUEST
)

type NetData struct {
  ID         string
  Request    int
  Response   int
  Msg        string // server msg or client chat msg
  Table      *Table
  PlayerData *Player
}

func (netData *NetData) Init() {
  return
}

// NOTE: tmp for debugging
func netDataReqToString(netData *NetData) string {
  if netData == nil {
    return "netData == nil"
  }

  netDataReqStringMap := map[int]string{
    NETDATA_CLOSE:        "NETDATA_CLOSE",
    NETDATA_NEWCONN:      "NETDATA_NEWCONN",
    NETDATA_YOURPLAYER:   "NETDATA_YOURPLAYER",
    NETDATA_NEWPLAYER:    "NETDATA_NEWPLAYER",
    NETDATA_CURPLAYERS:   "NETDATA_CURPLAYERS",
    NETDATA_UPDATEPLAYER: "NETDATA_UPDATEPLAYER",
    NETDATA_UPDATETABLE:  "NETDATA_UPDATETABLE",
    NETDATA_PLAYERLEFT:   "NETDATA_PLAYERLEFT",
    NETDATA_CLIENTEXITED: "NETDATA_CLIENTEXITED",
    NETDATA_RESET:        "NETDATA_RESET",

    NETDATA_MAKEADMIN: "NETDATA_MAKEADMIN",
    NETDATA_STARTGAME: "NETDATA_STARTGAME",

    NETDATA_CHATMSG: "NETDATA_CHATMSG",

    NETDATA_PLAYERACTION: "NETDATA_PLAYERACTION",
    NETDATA_PLAYERTURN:   "NETDATA_PLAYERTURN",
    NETDATA_ALLIN:        "NETDATA_ALLIN",
    NETDATA_BET:          "NETDATA_BET",
    NETDATA_CALL:         "NETDATA_CALL",
    NETDATA_CHECK:        "NETDATA_CHECK",
    NETDATA_RAISE:        "NETDATA_RAISE",
    NETDATA_FOLD:         "NETDATA_FOLD",
    NETDATA_CURHAND:      "NETDATA_CURHAND",
    NETDATA_SHOWHAND:     "NETDATA_SHOWHAND",

    NETDATA_FIRSTACTION:      "NETDATA_FIRSTACTION",
    NETDATA_MIDROUNDADDITION: "NETDATA_MIDROUNDADDITION",
    NETDATA_ELIMINATED:       "NETDATA_ELIMINATED",
    NETDATA_VACANTSEAT:       "NETDATA_VACANTSEAT",

    NETDATA_DEAL:      "NETDATA_DEAL",
    NETDATA_FLOP:      "NETDATA_FLOP",
    NETDATA_TURN:      "NETDATA_TURN",
    NETDATA_RIVER:     "NETDATA_RIVER",
    NETDATA_BESTHAND:  "NETDATA_BESTHAND",
    NETDATA_ROUNDOVER: "NETDATA_ROUNDOVER",

    NETDATA_SERVERMSG:  "NETDATA_SERVERMSG",
    NETDATA_BADREQUEST: "NETDATA_BADREQUEST",
  }

  if netDataStr, ok := netDataReqStringMap[netData.Request]; ok {
    return netDataStr
  }

  return "invalid NetData request"
}

func sendData(data *NetData, conn *websocket.Conn) {
  if data == nil {
    panic("sendData(): data == nil")
  }

  if conn == nil {
    panic("sendData(): websocket == nil")
  }

  // TODO: move this
  // XXX modifies global table
  /*if (data.Table != nil) {
    data.Table.Dealer     = data.Table.PublicPlayerInfo(*data.Table.Dealer)
    data.Table.SmallBlind = data.Table.PublicPlayerInfo(*data.Table.SmallBlind)
    data.Table.BigBlind   = data.Table.PublicPlayerInfo(*data.Table.BigBlind)
  }*/

  //fmt.Printf("sending %p to %p...\n", data, conn)

  var gobBuf bytes.Buffer
  enc := gob.NewEncoder(&gobBuf)

  enc.Encode(data)

  conn.WriteMessage(websocket.BinaryMessage, gobBuf.Bytes())
}

func serverCloseConn(conn *websocket.Conn) {
  fmt.Printf("<= closing conn to %s\n", conn.RemoteAddr().String())
  conn.Close()
}

func runServer(table *Table, addr string) (err error) {
  clients := make([]*websocket.Conn, 0)
  clientIDMap := make(map[*websocket.Conn]string)
  playerMap := make(map[*websocket.Conn]*Player)
  var tableAdmin *websocket.Conn

  sendResponseToAll := func(data *NetData, except *websocket.Conn) {
    for _, clientConn := range clients {
      if clientConn != except {
        sendData(data, clientConn)
      }
    }
  }

  getPlayerConn := func(player *Player) *websocket.Conn {
    for conn, p := range playerMap {
      if p.Name == player.Name {
        return conn
      }
    }

    return nil
  }

  removeClient := func(conn *websocket.Conn) {
    table.mtx.Lock()
    defer table.mtx.Unlock()

    clientIdx := -1
    for i, clientConn := range clients {
      if clientConn == conn {
        clientIdx = i
        break
      }
    }
    if clientIdx == -1 {
      fmt.Println("removeClient(): BUG: couldn't find a conn in clients slice")
      return
    } else {
      clients = append(clients[:clientIdx], clients[clientIdx+1:]...)
    }

    table.NumConnected--

    netData := &NetData{
      Response: NETDATA_CLIENTEXITED,
      Table:    table,
    }

    sendResponseToAll(netData, nil)
  }

  var gameOver func()
  removePlayerByConn := func(conn *websocket.Conn) {
    reset := false // XXX race condition guard
    noPlayersLeft := false // XXX race condition guard

    table.mtx.Lock()
    defer func() {
      if reset {
        if noPlayersLeft {
          table.reset(nil)
          sendResponseToAll(&NetData{
            Response: NETDATA_RESET,
            Table: table,
          }, nil)
        } else {
          table.finishRound()
          gameOver()
        }
      }
    }()
    defer table.mtx.Unlock()

    player := playerMap[conn]

    if player != nil { // else client was a spectator
      fmt.Printf("removing %s\n", player.Name)
      delete(playerMap, conn)

      table.activePlayers.RemovePlayer(player)
      table.curPlayers.RemovePlayer(player)

      player.Clear()

      netData := &NetData{
        Response:   NETDATA_PLAYERLEFT,
        Table:      table,
        PlayerData: player,
      }

      sendResponseToAll(netData, conn)

      table.NumPlayers--
      fmt.Printf("removePlByConn: NP==%v\n", table.NumPlayers)

      if table.NumPlayers < 2 {
        table.State = TABLESTATE_RESET
        reset = true
        if table.NumPlayers == 0 {
          noPlayersLeft = true
          tableAdmin = nil
        }
        return
      }

      if conn == tableAdmin {
        tableAdmin = getPlayerConn(table.activePlayers.node.Player)
        assert(tableAdmin != nil, "getPlayerConn(): couldn't find activePlayers head websocket")
        sendData(&NetData{Response: NETDATA_MAKEADMIN}, tableAdmin)
      }

      if table.Dealer != nil && player.Name == table.Dealer.Player.Name {
        table.Dealer = nil
      }
      if table.SmallBlind != nil && player.Name == table.SmallBlind.Player.Name {
        table.SmallBlind = nil
      }
      if table.BigBlind != nil && player.Name == table.BigBlind.Player.Name {
        table.BigBlind = nil
      }
    }
  }

  removePlayer := func(player *Player) {
    for conn, p := range playerMap {
      if p == player {
        removePlayerByConn(conn)

        return
      }
    }
  }

  sendPlayerTurn := func(conn *websocket.Conn) {
    if table.curPlayer == nil {
      return
    }

    netData := &NetData{
      Response:   NETDATA_PLAYERTURN,
      PlayerData: table.PublicPlayerInfo(*table.curPlayer.Player),
    }

    netData.PlayerData.Action.Action = NETDATA_PLAYERTURN

    sendData(netData, conn)
  }

  sendPlayerTurnToAll := func() {
    netData := &NetData{
      Response:   NETDATA_PLAYERTURN,
      PlayerData: table.PublicPlayerInfo(*table.curPlayer.Player),
    }

    netData.PlayerData.Action.Action = NETDATA_PLAYERTURN

    sendResponseToAll(netData, nil)
  }

  sendPlayerActionToAll := func(player *Player, conn *websocket.Conn) {
    fmt.Printf("%s action => %s\n", player.Name, player.ActionToString())

    netData := &NetData{
      Response:   NETDATA_PLAYERACTION,
      Table:      table,
      PlayerData: table.PublicPlayerInfo(*player),
    }

    sendResponseToAll(netData, conn)

    if conn != nil { // conn is nil for blind auto allin corner case
      netData.PlayerData = player
      sendData(netData, conn)
    }
  }

  sendDeals := func() {
    netData := &NetData{Response: NETDATA_DEAL}

    for conn, player := range playerMap {
      netData.PlayerData = player

      sendData(netData, conn)
    }
  }

  sendHands := func() {
    netData := &NetData{Response: NETDATA_SHOWHAND}

    for _, player := range table.curPlayers.ToPlayerArray() {
      netData.PlayerData = table.PublicPlayerInfo(*player)

      var conn *websocket.Conn
      for k, v := range playerMap {
        if v == player {
          conn = k
          break
        }
      }
      assert(conn != nil, "sendHands(): player not in playerMap")

      sendResponseToAll(netData, conn)
    }
  }

  sendTable := func() {
    netData := &NetData{
      Response: NETDATA_UPDATETABLE,
      Table:    table,
    }

    sendResponseToAll(netData, nil)
  }

  gameOver = func() {
    fmt.Printf("** game over %s wins **\n", table.Winners[0].Name)
    winner := table.Winners[0]

    netData := &NetData{
      Response: NETDATA_SERVERMSG,
      Msg:      "game over, " + winner.Name + " wins",
    }

    sendResponseToAll(netData, nil)

    table.reset(winner) // make a new game while keeping winner connected

    if winnerConn := getPlayerConn(winner); winnerConn != tableAdmin {
      if winnerConn == nil {
        fmt.Printf("getPlayerConn(): winner (%s) not found\n", winner.Name)
        return
      }
      tableAdmin = winnerConn
      sendData(&NetData{Response: NETDATA_MAKEADMIN}, winnerConn)
      sendPlayerTurnToAll()

      sendResponseToAll(&NetData{
        Response: NETDATA_RESET,
        PlayerData: winner,
        Table: table,
      }, nil)
    }
  }

  checkBlindsAutoAllIn := func() {
    if table.SmallBlind.Player.Action.Action == NETDATA_ALLIN {
      fmt.Printf("checkBlindsAutoAllIn(): smallblind (%s) forced to go all in\n", table.SmallBlind.Player.Name)

      if table.curPlayer.Player.Name == table.SmallBlind.Player.Name {
        // because blind is curPlayer setNextPlayerTurn() will remove the blind from the list for us
        table.setNextPlayerTurn()
      } else {
        table.curPlayers.RemovePlayer(table.SmallBlind.Player)
      }

      sendPlayerActionToAll(table.SmallBlind.Player, nil)
    }
    if table.BigBlind.Player.Action.Action == NETDATA_ALLIN {
      fmt.Printf("checkBlindsAutoAllIn(): bigblind (%s) forced to go all in\n", table.BigBlind.Player.Name)

      if table.curPlayer.Player.Name == table.BigBlind.Player.Name {
        // because blind is curPlayer setNextPlayerTurn() will remove the blind from the list for us
        table.setNextPlayerTurn()
      } else {
        table.curPlayers.RemovePlayer(table.BigBlind.Player)
      }

      sendPlayerActionToAll(table.BigBlind.Player, nil)
    }
  }

  roundOver := func() {
    table.finishRound()
    sendHands()

    netData := &NetData{
      Response: NETDATA_ROUNDOVER,
      Table:    table,
      Msg:      table.WinInfo,
    }

    for i, sidePot := range table.SidePot {
      netData.Msg += fmt.Sprintf("\nsidePot #%d:\n%s", i+1, sidePot.WinInfo)
    }

    sendResponseToAll(netData, nil)

    netData.Response = NETDATA_UPDATEPLAYER
    netData.Table, netData.Msg = nil, ""
    for _, player := range table.activePlayers.ToPlayerArray() {
      netData.PlayerData = player

      sendResponseToAll(netData, nil)
    }

    for _, player := range table.getEliminatedPlayers() {
      netData.Response = NETDATA_ELIMINATED
      netData.PlayerData = player

      removePlayer(player)
      sendResponseToAll(netData, nil)
    }

    if table.State == TABLESTATE_GAMEOVER {
      gameOver()

      return
    }

    table.newRound()
    table.nextTableAction()
    checkBlindsAutoAllIn()
    sendDeals()
    sendPlayerTurnToAll()
    sendTable()
  }

  // TODO: this is temporary.
  tmp_tableLogicAfterPlayerAction := func(player *Player, netData *NetData, conn *websocket.Conn) {
    if table.State != TABLESTATE_DONEBETTING {
      if table.State == TABLESTATE_ROUNDOVER {
        // all other players folded before all comm cards were dealt
        // TODO: check for this state in a better fashion
        table.finishRound()
        fmt.Printf("winner # %d\n", len(table.Winners))
        fmt.Println(table.Winners[0].Name + " wins by folds")

        netData.Response = NETDATA_ROUNDOVER
        netData.Table = table
        netData.Msg = table.Winners[0].Name + " wins by folds"
        netData.PlayerData = nil

        sendResponseToAll(netData, nil)

        for _, player := range table.getEliminatedPlayers() {
          netData.Response = NETDATA_ELIMINATED
          netData.Msg = ""
          netData.PlayerData = player

          removePlayer(player)
          sendResponseToAll(netData, nil)
        }

        if table.State == TABLESTATE_GAMEOVER {
          gameOver()

          return
        }

        table.newRound()
        table.nextTableAction()
        checkBlindsAutoAllIn()
        sendDeals()
        sendPlayerTurnToAll()
        sendTable()
      } else {
        sendPlayerActionToAll(player, conn)
        sendPlayerTurnToAll()
      }
    } else {
      sendPlayerActionToAll(player, conn)
      sendPlayerTurnToAll()

      fmt.Println("** done betting...")

      if table.bettingIsImpossible() {
        fmt.Println("2: no more betting possible...")

        for table.State != TABLESTATE_ROUNDOVER {
          table.nextCommunityAction()
        }
      } else {
        table.nextCommunityAction()
      }

      if table.State == TABLESTATE_ROUNDOVER {
        roundOver()

        if table.State == TABLESTATE_GAMEOVER {
          return // XXX
        }
      } else { // new community card(s)
        netData.Response = table.commState2NetDataResponse()
        netData.Table = table
        netData.PlayerData = nil

        sendResponseToAll(netData, nil)

        table.Bet, table.better = 0, nil
        for _, player := range table.curPlayers.ToPlayerArray() {
          fmt.Printf("clearing %v's action\n", player.Name)
          player.Action.Action = NETDATA_FIRSTACTION
          player.Action.Amount = 0
        }

        table.reorderPlayers()
        sendPlayerTurnToAll()

        // let players know they should update their current hand after
        // the community action
        // NOTE: hand is currently computed on client side
        netData.Response = NETDATA_CURHAND
        for conn, player := range playerMap {
          netData.PlayerData = player
          sendData(netData, conn)
        }
      }
    }
  }

  errChan := make(chan error)

  serverPanicked := false

  // cleanly close connections after a server panic()
  serverError := func(err error) {
    fmt.Println("server panicked")

    for _, conn := range clients {
      conn.WriteMessage(websocket.CloseMessage,
        websocket.FormatCloseMessage(websocket.CloseInternalServerErr,
          err.Error()))
    }

    errChan <- err

    serverPanicked = true
  }

  upgrader := websocket.Upgrader{}

  WSCLIClient := func(w http.ResponseWriter, req *http.Request) {
    if req.Header.Get("keepalive") != "" {
      return // NOTE: for heroku
    }

    conn, err := upgrader.Upgrade(w, req, nil)
    if err != nil {
      fmt.Printf("WS upgrade err %s\n", err.Error())

      return
    }

    cleanExit := false
    defer func() {
      if serverPanicked { // server panic was already recovered in previous client handler
        return
      }

      if err := recover(); err != nil {
        serverError(panicRetToError(err))
      } else { // not a server panic()
        if player := playerMap[conn]; player != nil {
          if !cleanExit {
            fmt.Printf("%s had an unclean exit\n", player.Name)
          }
          if table.activePlayers.len > 1 && table.curPlayer.Player.Name == player.Name {
            table.setNextPlayerTurn()
            sendPlayerTurnToAll()
          }
        }

        removeClient(conn)
        removePlayerByConn(conn)
        serverCloseConn(conn)
      }
    }()

    fmt.Printf("=> new conn from %s\n", req.Host)

    stopPing := make(chan bool)
    go func() {
      ticker := time.NewTicker(10 * time.Second)

      for {
        select {
        case <-stopPing:
          return
        case <-ticker.C:
          if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
            fmt.Printf("ping err: %s\n", err.Error())
            return
          }
        }
      }
    }()
    defer func() {
      stopPing <- true
    }()

    clientIDMap[conn] = randString(20)

    netData := NetData{
      ID:       clientIDMap[conn],
      Response: NETDATA_NEWCONN,
      Table:    table,
    }

    for {
      _, rawData, err := conn.ReadMessage()
      if err != nil {
        if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
          fmt.Printf("runServer(): readConn() conn: %v err: %v\n", conn, err)
        }

        return
      }

      // we need to set Table member to nil otherwise gob will
      // modify our table structure if a user sends that member
      netData = NetData{Response: NETDATA_NEWCONN, Table: nil}

      gob.NewDecoder(bufio.NewReader(bytes.NewReader(rawData))).Decode(&netData)

      netData.Table = table

      fmt.Printf("recv %s (%d bytes) from %p\n", netDataReqToString(&netData), len(rawData), conn)

      if netData.Request == NETDATA_NEWCONN {
        clients = append(clients, conn)

        table.mtx.Lock()
        table.NumConnected++
        table.mtx.Unlock()

        sendResponseToAll(&netData, nil)

        // send current player info to this client
        if table.NumConnected > 1 {
          netData.Response = NETDATA_CURPLAYERS
          netData.Table = table

          for _, player := range table.activePlayers.ToPlayerArray() {
            netData.PlayerData = table.PublicPlayerInfo(*player)
            sendData(&netData, conn)
          }
        }

        if player := table.getOpenSeat(); player != nil {
          fmt.Printf("adding %p as player %s\n", &conn, player.Name)

          if table.State == TABLESTATE_NOTSTARTED {
            player.Action.Action = NETDATA_FIRSTACTION
            table.curPlayers.AddPlayer(player)
          } else {
            player.Action.Action = NETDATA_MIDROUNDADDITION
          }
          table.activePlayers.AddPlayer(player)

          playerMap[conn] = player

          if table.curPlayer == nil {
            table.curPlayer = table.curPlayers.node
          }

          if table.Dealer == nil {
            table.Dealer = table.activePlayers.node
          } else if table.SmallBlind == nil {
            table.SmallBlind = table.Dealer.next
          } else if table.BigBlind == nil {
            table.BigBlind = table.SmallBlind.next
          }

          netData.Response = NETDATA_NEWPLAYER
          netData.Table = table
          netData.PlayerData = table.PublicPlayerInfo(*player)

          sendResponseToAll(&netData, conn)

          netData.ID = clientIDMap[conn]
          netData.Response = NETDATA_YOURPLAYER
          netData.PlayerData = player
          sendData(&netData, conn)
        } else {
          netData.Response = NETDATA_SERVERMSG
          netData.Msg = "No open seats available. You have been added as a spectator"

          sendData(&netData, conn)
        }

        sendPlayerTurn(conn)

        if tableAdmin == nil {
          table.mtx.Lock()
          tableAdmin = conn
          table.mtx.Unlock()

          sendData(&NetData{Response: NETDATA_MAKEADMIN}, conn)
        }
      } else {
        switch netData.Request {
        case NETDATA_CLIENTEXITED:
          cleanExit = true

          return
        case NETDATA_STARTGAME:
          if conn != tableAdmin {
            netData.Response = NETDATA_BADREQUEST
            netData.Msg = "only the table admin can do that"
            netData.Table = nil

            sendData(&netData, conn)
          } else if table.NumPlayers < 2 {
            netData.Response = NETDATA_BADREQUEST
            netData.Msg = "not enough players to start"
            netData.Table = nil

            sendData(&netData, conn)
          } else if table.State != TABLESTATE_NOTSTARTED {
            netData.Response = NETDATA_BADREQUEST
            netData.Msg = "this game has already started"
            netData.Table = nil

            sendData(&netData, conn)
          } else { // start game
            table.nextTableAction()

            sendDeals()
            sendPlayerTurnToAll()
            sendTable()
          }
        case NETDATA_CHATMSG:
          netData.ID = clientIDMap[conn]
          netData.Response = NETDATA_CHATMSG

          if len(netData.Msg) > 256 {
            netData.Msg = netData.Msg[:256] + "(snipped)"
          }

          if player := playerMap[conn]; player != nil {
            netData.Msg = fmt.Sprintf("[%s]: %s", player.Name, netData.Msg)
          } else {
            netData.Msg = fmt.Sprintf("[spectator]: %s", netData.Msg)
          }

          sendResponseToAll(&netData, nil)
        case NETDATA_ALLIN, NETDATA_BET, NETDATA_CALL, NETDATA_CHECK, NETDATA_FOLD:
          player := playerMap[conn]

          if player == nil {
            netData.Response = NETDATA_BADREQUEST
            netData.Msg = "you are not a player"
            netData.Table = nil

            sendData(&netData, conn)
            continue
          }

          if table.State == TABLESTATE_NOTSTARTED {
            netData.Response = NETDATA_BADREQUEST
            netData.Msg = "a game has not been started yet"
            netData.Table = nil

            sendData(&netData, conn)
            continue
          }

          if player.Name != table.curPlayer.Player.Name {
            netData.Response = NETDATA_BADREQUEST
            netData.Msg = "it's not your turn"
            netData.Table = nil

            sendData(&netData, conn)
            continue
          }

          if err := table.PlayerAction(player, netData.PlayerData.Action); err != nil {
            netData.Response = NETDATA_BADREQUEST
            netData.Table = nil
            netData.Msg = err.Error()

            sendData(&netData, conn)
          } else {
            tmp_tableLogicAfterPlayerAction(player, &netData, conn)
          }
        default:
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "bad request"
          netData.Table, netData.PlayerData = nil, nil

          sendData(&netData, conn)
        }
        //sendData(&netData, writeConn)
      } // else{} end
    } //for loop end
  } // func end

  fmt.Printf("starting server on %v\n", addr)

  server := &http.Server{
    Addr:        addr,
    IdleTimeout: 0,
    ReadTimeout: 0,
  }

  server.SetKeepAlivesEnabled(true)

  http.HandleFunc("/cli", WSCLIClient)

  go func() {
    if err := server.ListenAndServe(); err != nil {
      fmt.Printf("ListenAndServe(): %s\n", err.Error())
    }
  }()

  sigChan := make(chan os.Signal, 1)
  signal.Notify(sigChan, os.Interrupt)

  select {
  case sig := <-sigChan:
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    fmt.Fprintf(os.Stderr, "received signal: %s\n", sig.String())

    // TODO: ignore irrelevant signals
    sendResponseToAll(&NetData{Response: NETDATA_SERVERCLOSED}, nil)

    if err := server.Shutdown(ctx); err != nil {
      fmt.Fprintf(os.Stderr, "server.Shutdown(): %s\n", err.Error())
    }
  case err := <-errChan:
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    fmt.Fprintf(os.Stderr, "irrecoverable server error: %s\n", err.Error())

    if err := server.Shutdown(ctx); err != nil {
      fmt.Fprintf(os.Stderr, "server.Shutdown(): %s\n", err.Error())
    }
  }

  return nil
}

type FrontEnd interface {
  InputChan() chan *NetData
  OutputChan() chan *NetData
  Init() error
  Run() error
  Finish() chan error
  Error() chan error
}

func runClient(addr string, isGUI bool) (err error) {
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
      fmt.Fprintf(os.Stderr, "problem setting up keepalive request %s\n", err.Error())

      return
    }
    req.Header.Add("keepalive", "true")

    for {
      <-ticker.C

      _, err := client.Do(req)
      if err != nil {
        fmt.Fprintf(os.Stderr, "problem sending a keepalive request %s\n", err.Error())

        return
      }
    }
  }()

  defer func() {
    fmt.Fprintf(os.Stderr, "closing connection\n")

    sendData(&NetData{Request: NETDATA_CLIENTEXITED}, conn)

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

    sendData(&NetData{Request: NETDATA_NEWCONN}, conn)

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

    if err := runServer(table, "0.0.0.0:"+opts.serverMode); err != nil {
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
    if err := runClient(opts.connect, opts.gui); err != nil {
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
  gui        bool
  numSeats   uint
}

/*
  TODO: check if bets always have to be a multiple of blind(s)?
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
    gui        bool
    numSeats   uint
  )

  flag.Usage = func() {
    fmt.Println(usage)
    flag.PrintDefaults()
  }

  flag.StringVar(&serverMode, "s", "", "host a poker table on <port>")
  flag.StringVar(&connect, "c", "", "connect to a gopoker table")
  flag.BoolVar(&gui, "g", false, "run with a GUI")
  flag.UintVar(&numSeats, "ns", 7, "max number of players allowed at the table")
  flag.Parse()

  opts := &options{
    serverMode: serverMode,
    connect:    connect,
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
