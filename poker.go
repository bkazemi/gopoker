package main

import (
	"flag"
	"fmt"

	//"net"
	"bufio"
	"bytes"
	"encoding/gob"

	//"io"
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"time"

	//_ "net/http/pprof"

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
  defaultName string
  Name  string // NOTE: must have unique names
  IsCPU bool

  IsVacant bool

  ChipCount uint
  Hole      *Hole
  Hand      *Hand
  PreHand   Hand // XXX tmp
  Action    Action
}

func (p *Player) setName(name string) {
  oldName := p.Name
  if name == "" {
    p.Name = p.defaultName
  } else {
    p.Name = name
  }
  fmt.Printf("%s.setName(): '%s' => '%s'\n", p.defaultName, oldName, p.Name)
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
  player.defaultName = name
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
  player.Name = player.defaultName
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
  Total    uint
  Players  map[string]*Player
  IsClosed bool
  WinInfo  string
}

func (pot *Pot) Init() {
  pot.Players = make(map[string]*Player)
}

func (pot *Pot) HasPlayer(player *Player) bool {
  return pot.Players[player.Name] != nil
}

func (pot *Pot) AddPlayer(player *Player) {
  pot.Players[player.Name] = player
}

func (pot *Pot) RemovePlayer(player *Player) {
  if player == nil {
    printer.Printf("clearing <%v> bet sidePot\n", pot.Bet)
    pot.Players = make(map[string]*Player)
  } else {
    delete(pot.Players, player.Name)
  }
}

func (pot *Pot) Clear() {
  pot.Init()
  pot.Bet = 0
  pot.Total = 0
  pot.IsClosed = false
  pot.WinInfo = ""
}

type SidePot struct {
  Pot // XXX
}

func (sidePot *SidePot) Init(playerMap map[string]*Player, player *Player) {
  if playerMap == nil && player == nil {
    sidePot.Players = make(map[string]*Player)

    return
  }

  if playerMap != nil {
    sidePot.Players = make(map[string]*Player)

    for pName, p := range playerMap {
      sidePot.Players[pName] = p
    }

    if player != nil {
      if p := sidePot.Players[player.Name]; p == nil {
        sidePot.Players[player.Name] = player
      }
    }
  } else if player != nil {
    sidePot.Players = make(map[string]*Player)

    sidePot.Players[player.Name] = player
  }
}

type SidePotArray struct {
  Pots []*SidePot
}

func (arr *SidePotArray) Add(sidePot *SidePot) {
  arr.Pots = append(arr.Pots, sidePot)
}

func (arr *SidePotArray) Insert(sidePot *SidePot, idx int) {
  if idx < 0 || len(arr.Pots) != 0 && idx > len(arr.Pots) - 1 {
    panic(fmt.Sprintf("SidePotArray: Insert(): invalid index '%v'\n", idx))
  }

  if idx == 0 {
    arr.Pots = append([]*SidePot{sidePot}, arr.Pots...)
  } else {
    arr.Pots = append(append(arr.Pots[:idx], sidePot), arr.Pots[idx:]...)
  }
}

func (arr *SidePotArray) GetOpenPots() []*SidePot {
  openSidePots := make([]*SidePot, 0)

  for _, sidePot := range arr.Pots {
    if !sidePot.IsClosed {
      openSidePots = append(openSidePots, sidePot)
    }
  }

  return openSidePots
}

func (arr *SidePotArray) CloseAll() {
  for i, sidePot := range arr.GetOpenPots() {
    fmt.Printf("closing sidePot %d\n", i)
    sidePot.IsClosed = true
  }
}

func (arr *SidePotArray) IsEmpty() bool {
  return len(arr.Pots) == 0
}

type SidePots struct {
  AllInPots  SidePotArray
  BettingPot *SidePot
}

func (sidePots *SidePots) Init() {
  sidePots.AllInPots = SidePotArray{
    Pots: make([]*SidePot, 0),
  }
}

func (sidePots *SidePots) GetAllPots() []*SidePot {
  pots := make([]*SidePot, 0)

  for _, sidePot := range sidePots.AllInPots.Pots {
    pots = append(pots, sidePot)
  }
  if sidePots.BettingPot != nil {
    pots = append(pots, sidePots.BettingPot)
  }

  return pots
}

func (sidePots *SidePots) IsEmpty() bool {
  return sidePots.AllInPots.IsEmpty() && sidePots.BettingPot == nil
}

func (sidePots *SidePots) Clear() {
  sidePots.Init()
  sidePots.BettingPot = nil
}

func (sidePots *SidePots) CalculateTotals(mainPotBet uint) {
  allInCount := mainPotBet
  for _, sidePot := range sidePots.AllInPots.Pots {
    betDiff := sidePot.Bet
    if sidePot.Bet > allInCount {
      betDiff -= allInCount
    } else {
      allInCount = sidePot.Bet
    }
    sidePot.Total = betDiff * uint(len(sidePot.Players))
    printer.Printf("sidePots.CalculateTotals(): %v allin pot total set to %v\n",
                   sidePot.Bet, sidePot.Total)
    allInCount += sidePot.Bet
  }
}

func (sidePots *SidePots) Print() {
  for i, sidePot := range sidePots.AllInPots.Pots {
    printer.Printf("sp %v - bet: %v pot: %v closed: %v\n", i, sidePot.Bet,
                   sidePot.Total, sidePot.IsClosed)
    printer.Printf(" players: ")
    for p := range sidePot.Players {
      printer.Printf("%s, ", p)
    }
    fmt.Println()
  }

  if sidePot := sidePots.BettingPot; sidePot != nil {
    printer.Printf("sp betpot - bet: %v pot: %v closed: %v\n", sidePot.Bet,
                   sidePot.Total, sidePot.IsClosed)
    printer.Printf(" players: ")
    for p := range sidePot.Players {
      printer.Printf("%s, ", p)
    }
    fmt.Println()
  }
}

type Table struct {
  deck       *Deck // deck of cards
  Community  Cards // community cards
  _comsorted Cards // sorted community cards

  MainPot  *Pot     // table pot
  sidePots SidePots // sidepots for allins
  Ante     uint     // current ante TODO allow both ante & blind modes
  Bet      uint     // current bet

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
  table.MainPot.Init()

  table.sidePots = SidePots{}
  table.sidePots.Init()

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

  fmt.Println("reset(): resetting table")

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

  table.MainPot.Clear()

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
  return printer.Sprintf("%d chips", table.MainPot.Total)
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
  if len(table.sidePots.AllInPots.Pots) == 0 && table.allInCount() != 1 {
    return
  }

  if !table.MainPot.IsClosed {
    fmt.Printf("closeSidePots(): closing mainpot\n")
    table.MainPot.IsClosed = true
  }

  // XXX move me
  if table.allInCount() == 1 {
  // all players called the all-in player

  }

  table.sidePots.AllInPots.CloseAll()
}

func (table *Table) getChipLeaders(includeAllIns bool) (uint, uint) {
  if table.curPlayers.len < 2 {
    panic("BUG: getChipLeaders() called with < 2 non-folded/allin players")
  }

  var (
    chipLeader       uint
    secondChipLeader uint
  )

  var players []*Player
  if includeAllIns {
    players = table.getNonFoldedPlayers()
  } else {
    players = table.curPlayers.ToPlayerArray()
  }

  for _, p := range players {
    blindRequiredBet := uint(0)
    if p.Action.Amount == table.Ante || p.Action.Amount == table.Ante/2 {
      blindRequiredBet = p.Action.Amount
    }
    if p.ChipCount + (p.Action.Amount - blindRequiredBet) > chipLeader {
      chipLeader = p.ChipCount + (p.Action.Amount - blindRequiredBet)
    }
  }

  for _, p := range players {
    blindRequiredBet := uint(0)
    if p.Action.Amount == table.Ante || p.Action.Amount == table.Ante/2 {
      blindRequiredBet = p.Action.Amount
    }

    if p.ChipCount + (p.Action.Amount - blindRequiredBet) != chipLeader &&
       p.ChipCount + (p.Action.Amount - blindRequiredBet) > secondChipLeader {
      secondChipLeader = p.ChipCount + (p.Action.Amount - blindRequiredBet)
    }
  }

  if secondChipLeader == 0 { // all curPlayers have same chip count
    secondChipLeader = chipLeader
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
  } else {
    fmt.Printf("getEliminatedPlayers(): len ret: %v NP-1: %v\n", uint(len(ret)), table.NumPlayers-1)
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

  if table.BigBlind.next.Player.Name == table.Dealer.Player.Name {
    table.Dealer = table.BigBlind
  } else {
    table.Dealer = table.Dealer.next
  }
  table.SmallBlind = table.Dealer.next
  table.BigBlind = table.SmallBlind.next
}

func (table *Table) setNextPlayerTurn() {
  fmt.Printf("setNextPlayerTurn(): curP: %s\n", table.curPlayer.Player.Name)
  if table.State == TABLESTATE_NOTSTARTED {
    return
  }

  table.mtx.Lock()
  defer table.mtx.Unlock()

  Panic := &Panic{}
  Panic.Init()

  thisPlayer := table.curPlayer // save in case we need to remove from curPlayers list

  defer Panic.ifNoPanic(func() {
    if table.State == TABLESTATE_DONEBETTING {
      table.better = nil
      table.closeSidePots()
    }

    if thisPlayer.Player.Action.Action == NETDATA_ALLIN {
      nextNode := table.curPlayers.RemovePlayer(thisPlayer.Player)
      if nextNode != nil {
        fmt.Printf("setNextPl(): removing %s, curPs head is %s\n", thisPlayer.Player.Name,
                   table.curPlayers.node.Player.Name)
        table.curPlayer = nextNode
        fmt.Printf("  now %s\n", table.curPlayers.node.Player.Name)
      }
    }

    fmt.Printf("setNextPlayerTurn(): new curP: %v\n", table.curPlayer.Player.Name)
    table.curPlayers.ToPlayerArray()
  })

  if table.curPlayers.len == 1 {
    fmt.Println("setNextPlayerTurn(): table.curPlayers.len == 1")
    if table.allInCount() == 0 { // win by folds
      fmt.Println("allInCount == 0")
      table.State = TABLESTATE_ROUNDOVER // XXX
    } else {
      table.State = TABLESTATE_DONEBETTING
    }

    return
  }

  if thisPlayer.Player.Action.Action == NETDATA_FOLD {
    nextNode := table.curPlayers.RemovePlayer(thisPlayer.Player)
    if nextNode != nil {
      fmt.Printf("setNextPl(): removing %s, curPs head is %s\n", thisPlayer.Player.Name,
                 table.curPlayers.node.Player.Name)
      table.curPlayer = nextNode
      fmt.Printf("  now %s\n", table.curPlayers.node.Player.Name)
    }
  } else {
    table.curPlayer = thisPlayer.next
  }

  if table.curPlayers.len == 1 && table.allInCount() == 0 {
    fmt.Println("setNextPlayerTurn(): table.curPlayers.len == 1 with aiCnt of 0 after fold")
    table.State = TABLESTATE_ROUNDOVER // XXX

    return
  } else if thisPlayer.next == table.curPlayers.node &&
            table.curPlayers.node.Player.Action.Action != NETDATA_FIRSTACTION &&
            thisPlayer.Player.Action.Action != NETDATA_BET {
    // NOTE: curPlayers head gets shifted with allins / folds so we check for
    //       firstaction
    fmt.Printf("setNextPlayerTurn(): last player (%s) didn't raise\n", thisPlayer.Player.Name)
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

  if table.CommState == TABLESTATE_PREFLOP &&
     table.State != TABLESTATE_PLAYERRAISED { // XXX mixed states...
    if table.SmallBlind != nil && player.Name == table.SmallBlind.Player.Name {
      isSmallBlindPreFlop = true
      blindRequiredBet = minUInt(table.SmallBlind.Player.ChipCount, table.Ante / 2)
    } else if table.BigBlind!= nil && player.Name == table.BigBlind.Player.Name {
      blindRequiredBet = minUInt(table.BigBlind.Player.ChipCount, table.Ante)
    }
  }

  handleSidePots := func() {
    if table.sidePots.IsEmpty() { // first sidePot
      sidePot := &SidePot{
        Pot{
          Bet: table.Bet - player.Action.Amount,
        },
      }
      sidePot.Init(nil, nil)

      if sidePot.Bet == 0 { // first allin was a raise/exact match bet
        fmt.Printf("is1stSp: <%s> allin created an empty betting sidepot\n", player.Name)
      } else {
        // get players who already called the last bet,
        // sub the delta of the last bet and this players
        // chipcount in mainpot, then add them to the mainpot & sidepot.
        for playerNode := table.curPlayers.node;
            playerNode.Player.Name != player.Name;
            playerNode = playerNode.next {
          p := playerNode.Player
          if p.Name == player.Name {
            break
          }

          table.MainPot.Total -= sidePot.Bet

          printer.Printf("is1stSP: <%s> sub %d from mainpot, add same amt to sidePot\n",
                          p.Name, sidePot.Bet)

          sidePot.Total += sidePot.Bet
          sidePot.AddPlayer(p)
        }
      }
      table.sidePots.BettingPot = sidePot

      table.MainPot.Bet = player.Action.Amount
      table.MainPot.Total += table.MainPot.Bet
      table.MainPot.AddPlayer(player)

      return
    }

    if player.Action.Action == NETDATA_ALLIN {
      if !table.MainPot.IsClosed {
        if table.MainPot.Bet > player.Action.Amount {
          printer.Printf("<%s> moving previous mainpot to first sidepot with bet of %v\n", player.Name,
                         table.MainPot.Bet)
          sidePot := &SidePot{
            Pot{
              Bet: table.MainPot.Bet,
            },
          }
          betDiff := table.MainPot.Bet - player.Action.Amount
          printer.Printf("<%s> changed mainpot bet from %v to %v betDiff %v\n", player.Name,
                     table.MainPot.Bet, player.Action.Amount, betDiff)
          table.MainPot.Bet = player.Action.Amount
          table.MainPot.Total -= betDiff * uint(len(table.MainPot.Players))

          sidePot.Init(table.MainPot.Players, nil)
          table.sidePots.AllInPots.Insert(sidePot, 0)
          table.MainPot.AddPlayer(player)
        } else if table.MainPot.Bet == player.Action.Amount {
          printer.Printf("<%s> allin (%v) matched the mainpot allin\n", player.Name, player.Action.Amount)
          table.MainPot.AddPlayer(player)
          table.MainPot.Total += table.MainPot.Bet
        } else {
          idx, allInAmount := -1, table.MainPot.Bet
          for i, sidePot := range table.sidePots.AllInPots.Pots {
            if sidePot.IsClosed {
              continue
            }

            allInAmount += sidePot.Bet - allInAmount
            if allInAmount <= player.Action.Amount {
              sidePot.AddPlayer(player)
              if allInAmount == player.Action.Amount {
                idx--
                break
              }
            } else {
              idx = i
              break
            }
          }
          switch idx {
          case -2:
            fmt.Printf("<%s> all in matched a previous all in sidePot\n", player.Name)
          case -1:
            betDiff := table.sidePots.BettingPot.Bet - player.Action.Amount

            printer.Printf("<%s> %v all in is largest AllIn sidePot\n",
                          player.Name, player.ChipCountToString())
            printer.Printf("bettingpot bet changed from %v to %v\n",
                           table.sidePots.BettingPot.Bet,
                           table.sidePots.BettingPot.Bet - player.Action.Amount)
            printer.Printf("bettingpot pot changed from %v to %v\n",
                           table.sidePots.BettingPot.Total,
                           table.sidePots.BettingPot.Bet - (betDiff * uint(len(table.sidePots.BettingPot.Players))))

            if !table.MainPot.HasPlayer(player) {
              fmt.Printf("<%s> adding to mainpot\n", player.Name)
              table.MainPot.AddPlayer(player)
              table.MainPot.Total += table.MainPot.Bet
            }

            sidePot := &SidePot{
              Pot{
                Bet: player.Action.Amount,
              },
            }
            sidePot.Init(nil, player)
            table.sidePots.AllInPots.Add(sidePot)

            table.sidePots.BettingPot.Bet -= betDiff
            table.sidePots.BettingPot.Total -= betDiff * uint(len(table.sidePots.BettingPot.Players))
          default:
            if !table.MainPot.HasPlayer(player) {
              fmt.Printf("<%s> adding to mainpot\n", player.Name)
              table.MainPot.AddPlayer(player)
              table.MainPot.Total += table.MainPot.Bet
            }

            printer.Printf("<%s> inserting %v allin at idx %v\n", player.Name,
                           player.ChipCountToString(), idx)
            sidePot := &SidePot{
              Pot{
                Bet: player.Action.Amount,
              },
            }
            sidePot.Init(nil, player)
            table.sidePots.AllInPots.Insert(sidePot, idx)
          }
        }
      } else { // mainpot closed
        idx, allInAmount := -1, uint(0)
        for i, sidePot := range table.sidePots.AllInPots.Pots {
          if sidePot.IsClosed {
            continue
          }

          allInAmount += sidePot.Bet - allInAmount
          if allInAmount <= player.Action.Amount {
            sidePot.Players[player.Name] = player
            if allInAmount == player.Action.Amount {
              idx--
              break
            }
          } else {
            idx = i
            break
          }
        }
        switch idx {
        case -2:
          fmt.Printf("mpclosed: <%s> all in matched a previous all in sidePot\n", player.Name)
        case -1:
          betDiff := table.sidePots.BettingPot.Bet - player.Action.Amount
          if betDiff == 0 {
            betDiff = player.Action.Amount
          }

          printer.Printf("mpclosed: <%s> %v all in is largest AllIn sidePot\n",
                        player.Name, player.ChipCountToString())
          printer.Printf("mpclosed: bettingpot bet changed from %v to %v\n",
                         table.sidePots.BettingPot.Bet,
                         table.sidePots.BettingPot.Bet - player.Action.Amount)
          printer.Printf("mpclosed: bettingpot pot changed from %v to %v\n",
                         table.sidePots.BettingPot.Total,
                         table.sidePots.BettingPot.Bet - (betDiff * uint(len(table.sidePots.BettingPot.Players))))

          sidePot := &SidePot{
            Pot{
              Bet: player.Action.Amount,
            },
          }
          if sidePot.Bet > table.sidePots.BettingPot.Bet {
            sidePot.Init(nil, player)
          } else {
            sidePot.Init(table.sidePots.BettingPot.Players, nil)
          }
          table.sidePots.AllInPots.Add(sidePot)

          table.sidePots.BettingPot.Bet -= betDiff
          table.sidePots.BettingPot.Total -= betDiff * uint(len(table.sidePots.BettingPot.Players))
        default:
          printer.Printf("mpclosed: <%s> inserting %v allin at idx %v\n", player.Name,
                         player.ChipCountToString(), idx)
          sidePot := &SidePot{
            Pot{
              Bet: player.Action.Amount,
            },
          }
          sidePot.Init(nil, player)
          table.sidePots.AllInPots.Insert(sidePot, idx)
        }
      }
    } else { // not an allin
      if !table.MainPot.IsClosed && !table.MainPot.HasPlayer(player) {
        assert(player.ChipCount >= table.MainPot.Bet,
               printer.Sprintf("<%v> cc: %v cant match mainpot bet %v",
                               player.Name, player.ChipCount, table.MainPot.Bet))

        table.MainPot.Total += table.MainPot.Bet
        table.MainPot.Players[player.Name] = player
      }

      if !table.sidePots.BettingPot.HasPlayer(player) {
        table.sidePots.BettingPot.AddPlayer(player)
      }

      // add current player to open sidepots. this happens when multiple
      // players go all-in.
      for _, sidePot := range table.sidePots.AllInPots.GetOpenPots() {
        assert(player.ChipCount >= sidePot.Bet, "player cant match a sidePot bet")

        if !sidePot.HasPlayer(player) {
          sidePot.Players[player.Name] = player
          printer.Printf("adding %s to open sidePot (%v bet)\n", player.Name, sidePot.Bet)
        } else {
          printer.Printf("%s already in sidePot (%v bet)\n", player.Name, sidePot.Bet)
        }
      }

      bettingPot := table.sidePots.BettingPot
      switch player.Action.Action {
      case NETDATA_BET:
        lspb := table.sidePots.BettingPot.Bet
        if table.State == TABLESTATE_PLAYERRAISED &&
          player.Action.Amount > bettingPot.Bet {
          fmt.Printf("bettingpot: %s re-raised\n", player.Name)
          if bettingPot.Bet == 0 {
            fmt.Printf("bettingpot: %s re-raised an all-in\n", player.Name)
          }
          bettingPot.Total += player.Action.Amount - bettingPot.Bet
          bettingPot.Bet = player.Action.Amount
        } else {
          fmt.Printf("bettingpot: %s made new bet\n", player.Name)
          bettingPot.Bet = player.Action.Amount
          bettingPot.Total += bettingPot.Bet
        }
        if bettingPot.Bet != lspb {
          printer.Printf("bettingpot: %s changed betting pot bet from %d to %d\n", player.Name,
            lspb, player.Action.Amount)
        }
      case NETDATA_CALL:
        fmt.Printf("bettingpot: %s called\n", player.Name)
        bettingPot.Total += bettingPot.Bet
        bettingPot.AddPlayer(player)
      }
    }
  }

  if table.curPlayers.len == 1 &&
     (action.Action == NETDATA_ALLIN || action.Action == NETDATA_BET) {
    return errors.New(printer.Sprintf("you must call the raise (%d chips) or fold", table.Bet))
  }

  if player.ChipCount == 0 && action.Action != NETDATA_ALLIN {
    fmt.Printf("PlayerAction(): changing %s's all-in bet to an allin action\n", player.Name)
    action.Action = NETDATA_ALLIN
  }

  switch action.Action {
  case NETDATA_ALLIN:
    player.Action.Action = NETDATA_ALLIN

    // we need to add the blind's chips back, otherwise it would get added to current bet
    //player.Action.Amount -= blindRequiredBet
    //player.ChipCount += blindRequiredBet

    prevChips := player.Action.Amount
    printer.Printf("PlayerAction(): ALLIN: %s prevChips == %v\n", player.Name, prevChips)

    player.Action.Amount += prevChips
    player.ChipCount += prevChips

    if table.bettingIsImpossible() {
      fmt.Printf("PlayerAction(): last player (%s) went all-in\n", player.Name)
      player.Action.Amount = minUInt(table.Bet, player.ChipCount)
    } else {
      chipLeaderCount, secondChipLeaderCount := table.getChipLeaders(true)

      // NOTE: A chipleader can only bet what at least one other player can match.
      if player.ChipCount > table.Bet && player.ChipCount == chipLeaderCount {
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
    }

    handleSidePots()

    player.ChipCount -= player.Action.Amount

    //table.curPlayers.RemovePlayer(player)
  case NETDATA_BET:
    prevChips := player.Action.Amount
    printer.Printf("PlayerAction(): BET: %s prevChips == %v\n", player.Name, prevChips)

    if action.Amount < table.Ante {
      return errors.New(printer.Sprintf("bet must be greater than the ante (%d chips)", table.Ante))
    } else if action.Amount <= table.Bet {
      return errors.New(printer.Sprintf("bet must be greater than the current bet (%d chips)", table.Bet))
    } else if action.Amount > player.ChipCount + prevChips {
      return errors.New("not enough chips")
    }

    // we need to add the blind's chips back, otherwise it would get added to current bet
    /*fmt.Printf("%s bRB: %v\n", player.Name, blindRequiredBet)
    player.Action.Amount -= blindRequiredBet
    player.ChipCount += blindRequiredBet
    table.MainPot.Total -= blindRequiredBet*/

    chipLeaderCount, secondChipLeaderCount := table.getChipLeaders(true)

    fmt.Printf("cLC: %v scLC: %v\n", chipLeaderCount, secondChipLeaderCount)

    printer.Printf("PlayerAction(): BET: %s adding prevChips %v\n", player.Name, prevChips)
    player.ChipCount += prevChips

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

    if player.Action.Action == NETDATA_ALLIN || !table.sidePots.IsEmpty() {
      handleSidePots()
    } else {
      table.MainPot.Total += player.Action.Amount - prevChips
    }

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
    /*player.Action.Amount -= blindRequiredBet
    player.ChipCount += blindRequiredBet
    table.MainPot.Total -= blindRequiredBet*/

    prevChips := player.Action.Amount - blindRequiredBet
    printer.Printf("PlayerAction(): CALL: %s prevChips == %v\n", player.Name, prevChips)

    //if blindRequiredBet != 0 && table.Bet > table.Ante {
    //  player.Action.Amount -= blindRequiredBet
    //}

    player.ChipCount += prevChips

    // delta of bet & curPlayer's last bet
    betDiff := table.Bet - player.Action.Amount

    fmt.Printf("%s betDiff: %v\n", player.Name, betDiff)

    if table.Bet >= player.ChipCount {
      player.Action.Action = NETDATA_ALLIN
      player.Action.Amount = player.ChipCount

      handleSidePots()

      player.ChipCount = 0
    } else {
      player.Action.Action = NETDATA_CALL
      player.Action.Amount = table.Bet - blindRequiredBet

      if !table.sidePots.IsEmpty() {
        handleSidePots()
      } else {
        table.MainPot.Total += betDiff
      }
      player.ChipCount -= player.Action.Amount
    }
  case NETDATA_CHECK:
    if table.State == TABLESTATE_PLAYERRAISED {
      return errors.New(printer.Sprintf("you must call the raise (%d chips)", table.Bet))
    }

    if isSmallBlindPreFlop {
      return errors.New(printer.Sprintf("you must call the big blind (+%d chips)", blindRequiredBet))
    }

    if player.ChipCount == 0 { // big blind had a chipcount <= ante
      player.Action.Action = NETDATA_ALLIN
    } else {
      player.Action.Action = NETDATA_CHECK
    }
  case NETDATA_FOLD:
    player.Action.Action = NETDATA_FOLD

    //table.curPlayers.RemovePlayer(player)
  default:
    return errors.New("BUG: invalid player action: " + player.ActionToString())
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

    table.SmallBlind.Player.Action.Amount = minUInt(table.Ante/2,
                                                    table.SmallBlind.Player.ChipCount)
    table.BigBlind.Player.Action.Amount = minUInt(table.Ante,
                                                  table.BigBlind.Player.ChipCount)

    table.SmallBlind.Player.ChipCount -= table.SmallBlind.Player.Action.Amount
    table.BigBlind.Player.ChipCount -= table.BigBlind.Player.Action.Amount

    table.MainPot.Total = table.SmallBlind.Player.Action.Amount + table.BigBlind.Player.Action.Amount

    table.Deal()

    table.CommState = TABLESTATE_PREFLOP

    table.reorderPlayers() // NOTE: need to call this to properly set curPlayer
  case TABLESTATE_NEWROUND:
    table.rotatePlayers()

    table.Bet = table.Ante

    table.SmallBlind.Player.Action.Amount = minUInt(table.Ante/2,
                                                    table.SmallBlind.Player.ChipCount)
    table.BigBlind.Player.Action.Amount = minUInt(table.Ante,
                                                  table.BigBlind.Player.ChipCount)

    table.SmallBlind.Player.ChipCount -= table.SmallBlind.Player.Action.Amount
    table.BigBlind.Player.ChipCount -= table.BigBlind.Player.Action.Amount

    table.MainPot.Total = table.SmallBlind.Player.Action.Amount + table.BigBlind.Player.Action.Amount

    if table.SmallBlind.Player.ChipCount == 0 {
      table.SmallBlind.Player.Action.Action = NETDATA_ALLIN
    }
    if table.BigBlind.Player.ChipCount == 0 {
      table.BigBlind.Player.Action.Action = NETDATA_ALLIN
    }

    table.MainPot.Total = table.SmallBlind.Player.Action.Amount + table.BigBlind.Player.Action.Amount

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
  table.MainPot.Clear()
  table.MainPot.Bet = table.Bet
  table.sidePots.Clear()
  table.State = TABLESTATE_NEWROUND
}

func (table *Table) finishRound() {
  table.mtx.Lock()
  defer table.mtx.Unlock()
  // special case for when everyone except a folded player
  // leaves the table
  if table.activePlayers.len == 1 &&
     table.activePlayers.node.Player.Action.Action == NETDATA_FOLD {
    fmt.Printf("finishRound(): only one folded player (%s) left at table. " +
               "abandoning all pots\n",
               table.activePlayers.node.Player.Name)

    table.State = TABLESTATE_GAMEOVER
    table.Winners = []*Player{table.activePlayers.node.Player}

    return
  }

  players := table.getNonFoldedPlayers()

  printer.Printf("mainpot: %d\n", table.MainPot.Total)
  table.sidePots.CalculateTotals(table.MainPot.Bet)
  table.sidePots.Print()
  if table.sidePots.BettingPot != nil &&
     table.sidePots.BettingPot.Bet == 0 {
      fmt.Printf("removing empty bettingpot\n")
      table.sidePots.BettingPot = nil
  }

  if len(players) == 1 { // win by folds
    player := players[0]

    player.ChipCount += table.MainPot.Total

    assert(table.sidePots.IsEmpty(),
      printer.Sprintf("BUG: finishRound(): %s won by folds but there are sidepots", player.Name))

    table.State = TABLESTATE_ROUNDOVER
    table.Winners = players

    return
  }

  table.State = TABLESTATE_SHOWHANDS

  bestPlayers := table.BestHand(players, nil)

  // TODO: redundant code
  if len(bestPlayers) == 1 {
    bestPlayers[0].ChipCount += table.MainPot.Total
  } else {
    splitChips := table.MainPot.Total / uint(len(bestPlayers))

    printer.Printf("mainpot: split chips: %v\n", splitChips)

    for _, player := range bestPlayers {
      player.ChipCount += splitChips
    }

    table.State = TABLESTATE_SPLITPOT
  }

  table.Winners = bestPlayers

  for i, sidePot := range table.sidePots.GetAllPots() {
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

      player.ChipCount += sidePot.Total

      table.Winners = append(table.Winners, player)
    } else {
      sidePotPlayersArr := make([]*Player, 0, len(sidePot.Players))
      for _, player := range sidePot.Players { // TODO: make a mapval2slice util func
        sidePotPlayersArr = append(sidePotPlayersArr, player)
      }
      bestPlayers := table.BestHand(sidePotPlayersArr, sidePot)

      if len(bestPlayers) == 1 {
        fmt.Printf("%s won sidePot #%d\n", bestPlayers[0].Name, i)
        bestPlayers[0].ChipCount += sidePot.Total
      } else {
        splitChips := sidePot.Total / uint(len(bestPlayers))

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
  NETDATA_CLIENTSETTINGS
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

  ClientSettings *ClientSettings // client requested settings
  Table          *Table
  PlayerData     *Player
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
    NETDATA_CLOSE:          "NETDATA_CLOSE",
    NETDATA_NEWCONN:        "NETDATA_NEWCONN",
    NETDATA_YOURPLAYER:     "NETDATA_YOURPLAYER",
    NETDATA_NEWPLAYER:      "NETDATA_NEWPLAYER",
    NETDATA_CURPLAYERS:     "NETDATA_CURPLAYERS",
    NETDATA_UPDATEPLAYER:   "NETDATA_UPDATEPLAYER",
    NETDATA_UPDATETABLE:    "NETDATA_UPDATETABLE",
    NETDATA_PLAYERLEFT:     "NETDATA_PLAYERLEFT",
    NETDATA_CLIENTEXITED:   "NETDATA_CLIENTEXITED",
    NETDATA_CLIENTSETTINGS: "NETDATA_CLIENTSETTINGS",
    NETDATA_RESET:          "NETDATA_RESET",

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

  // XXX remove me
  reqOrRes := NETDATA_CLOSE
  if netData.Request == NETDATA_CLOSE {
    reqOrRes = netData.Response
  } else {
    reqOrRes = netData.Request
  }

  if netDataStr, ok := netDataReqStringMap[reqOrRes]; ok {
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

type Server struct {
  clients []*websocket.Conn
  clientIDMap map[*websocket.Conn]string
  playerMap map[*websocket.Conn]*Player
  table *Table
  tableAdmin *websocket.Conn

  http *http.Server
  upgrader websocket.Upgrader

  sigChan chan os.Signal
  errChan chan error
  panicked bool
}

func (server *Server) Init(table *Table, addr string) error {
  server.clients = make([]*websocket.Conn, 0)
  server.clientIDMap = make(map[*websocket.Conn]string)
  server.playerMap = make(map[*websocket.Conn]*Player)
  server.table = table

  server.errChan = make(chan error)
  server.panicked = false

  server.upgrader = websocket.Upgrader{}

  server.http = &http.Server{
    Addr:        addr,
    IdleTimeout: 0,
    ReadTimeout: 0,
  }
  server.http.SetKeepAlivesEnabled(true)
  http.HandleFunc("/cli", server.WSCLIClient)

  server.sigChan = make(chan os.Signal, 1)
  signal.Notify(server.sigChan, os.Interrupt)

  return nil
}

func (server *Server) closeConn(conn *websocket.Conn) {
  fmt.Printf("<= closing conn to %s\n", conn.RemoteAddr().String())
  conn.Close()
}

func (server *Server) sendResponseToAll(data *NetData, except *websocket.Conn) {
  for _, clientConn := range server.clients {
    if clientConn != except {
      sendData(data, clientConn)
    }
  }
}

func (server *Server) getPlayerConn(player *Player) *websocket.Conn {
  for conn, p := range server.playerMap {
    if p.Name == player.Name {
      return conn
    }
  }

  return nil
}

func (server *Server) removeClient(conn *websocket.Conn) {
  server.table.mtx.Lock()
  defer server.table.mtx.Unlock()

  clientIdx := -1
  for i, clientConn := range server.clients {
    if clientConn == conn {
      clientIdx = i
      break
    }
  }
  if clientIdx == -1 {
    fmt.Println("removeClient(): BUG: couldn't find a conn in clients slice")
    return
  } else {
    server.clients = append(server.clients[:clientIdx], server.clients[clientIdx+1:]...)
  }

  server.table.NumConnected--

  netData := &NetData{
    Response: NETDATA_CLIENTEXITED,
    Table:    server.table,
  }

  server.sendResponseToAll(netData, nil)
}

func (server *Server) removePlayerByConn(conn *websocket.Conn) {
  reset := false // XXX race condition guard
  noPlayersLeft := false // XXX race condition guard

  server.table.mtx.Lock()
  defer func() {
    if server.table.State == TABLESTATE_NOTSTARTED {
      return
    }
    if reset {
      if noPlayersLeft {
        server.table.reset(nil)
        server.sendResponseToAll(&NetData{
          Response: NETDATA_RESET,
          Table: server.table,
        }, nil)
      } else {
        if server.table.State != TABLESTATE_ROUNDOVER &&
           server.table.State != TABLESTATE_GAMEOVER {
          fmt.Println("rPlByConn: state != (rndovr || gameovr)")
          server.table.finishRound()
          server.table.State = TABLESTATE_GAMEOVER
          server.gameOver()
        } else {
          fmt.Println("rPlByConn: state == rndovr || gameovr")
          //server.table.finishRound()
          //server.table.State = TABLESTATE_GAMEOVER
          //server.gameOver()
        }
      }
    } else if server.table.State == TABLESTATE_DONEBETTING ||
              server.table.State == TABLESTATE_ROUNDOVER {
      fmt.Println("removePl defer postPlAct")
      server.postPlayerAction(nil, &NetData{}, nil)
    }
  }()
  defer server.table.mtx.Unlock()

  player := server.playerMap[conn]

  if player != nil { // else client was a spectator
    fmt.Printf("removePlayerByConn(): removing %s\n", player.Name)
    delete(server.playerMap, conn)

    server.table.activePlayers.RemovePlayer(player)
    server.table.curPlayers.RemovePlayer(player)

    player.Clear()

    netData := &NetData{
      ID:         server.clientIDMap[conn],
      Response:   NETDATA_PLAYERLEFT,
      Table:      server.table,
      PlayerData: player,
    }

    server.sendResponseToAll(netData, conn)

    server.table.NumPlayers--

    if server.table.NumPlayers < 2 {
      reset = true
      if server.table.NumPlayers == 0 {
        noPlayersLeft = true
        server.tableAdmin = nil
      }
      return
    }

    if conn == server.tableAdmin {
      server.tableAdmin = server.getPlayerConn(server.table.activePlayers.node.Player)
      assert(server.tableAdmin != nil,
             "getPlayerConn(): couldn't find activePlayers head websocket")
      sendData(&NetData{Response: NETDATA_MAKEADMIN}, server.tableAdmin)
    }

    if server.table.Dealer != nil &&
       player.Name == server.table.Dealer.Player.Name {
      server.table.Dealer = nil
    }
    if server.table.SmallBlind != nil &&
       player.Name == server.table.SmallBlind.Player.Name {
      server.table.SmallBlind = nil
    }
    if server.table.BigBlind != nil &&
       player.Name == server.table.BigBlind.Player.Name {
      server.table.BigBlind = nil
    }
  }
}

func (server *Server) removePlayer (player *Player) {
  for conn, p := range server.playerMap {
    if p == player {
      server.removePlayerByConn(conn)

      return
    }
  }
}

func (server *Server) sendPlayerTurn(conn *websocket.Conn) {
  if server.table.curPlayer == nil {
    return
  }

  curPlayer := server.table.curPlayer.Player
  id := server.clientIDMap[server.getPlayerConn(curPlayer)]

  netData := &NetData{
    ID:         id,
    Response:   NETDATA_PLAYERTURN,
    PlayerData: server.table.PublicPlayerInfo(*curPlayer),
  }

  netData.PlayerData.Action.Action = NETDATA_PLAYERTURN

  sendData(netData, conn)
}

func (server *Server) sendPlayerTurnToAll() {
  if server.table.curPlayer == nil {
    return
  }

  curPlayer := server.table.curPlayer.Player
  id := server.clientIDMap[server.getPlayerConn(curPlayer)]

  netData := &NetData{
    ID:         id,
    Response:   NETDATA_PLAYERTURN,
    PlayerData: server.table.PublicPlayerInfo(*curPlayer),
  }

  netData.PlayerData.Action.Action = NETDATA_PLAYERTURN

  server.sendResponseToAll(netData, nil)
}

func (server *Server) sendPlayerActionToAll(player *Player, conn *websocket.Conn) {
  fmt.Printf("%s action => %s\n", player.Name, player.ActionToString())

  var c *websocket.Conn
  if conn == nil {
    c = server.getPlayerConn(player)
  } else {
    c = conn
  }

  netData := &NetData{
    ID:         server.clientIDMap[c],
    Response:   NETDATA_PLAYERACTION,
    Table:      server.table,
    PlayerData: server.table.PublicPlayerInfo(*player),
  }

  server.sendResponseToAll(netData, conn)

  if conn != nil { // conn is nil for blind auto allin corner case
    netData.PlayerData = player
    sendData(netData, conn)
  }
}

func (server *Server) sendDeals() {
  netData := &NetData{Response: NETDATA_DEAL}

  for conn, player := range server.playerMap {
    netData.ID = server.clientIDMap[conn]
    netData.PlayerData = player
    netData.Table = server.table

    sendData(netData, conn)
  }
}

func (server *Server) sendHands() {
  netData := &NetData{Response: NETDATA_SHOWHAND, Table: server.table}

  for _, player := range server.table.curPlayers.ToPlayerArray() {
    conn := server.getPlayerConn(player)
    assert(conn != nil, "sendHands(): player not in playerMap")
    netData.ID = server.clientIDMap[conn]
    netData.PlayerData = server.table.PublicPlayerInfo(*player)

    server.sendResponseToAll(netData, conn)
  }
}

// NOTE: hand is currently computed on client side
func (server *Server) sendCurHands() {
  netData := &NetData{Response: NETDATA_CURHAND, Table: server.table}

  for conn, player := range server.playerMap {
    netData.ID = server.clientIDMap[conn]
    netData.PlayerData = player
    sendData(netData, conn)
  }
}

func (server *Server) sendAllPlayerInfo(curPlayers bool) {
  netData := &NetData{Response: NETDATA_UPDATEPLAYER}

  var players playerList
  if curPlayers {
    players = server.table.curPlayers
  } else {
    players = server.table.activePlayers
  }

  for _, player := range players.ToPlayerArray() {
    conn := server.getPlayerConn(player)
    netData.ID = server.clientIDMap[conn]
    netData.PlayerData = server.table.PublicPlayerInfo(*player)

    server.sendResponseToAll(netData, conn)

    netData.PlayerData = player
    sendData(netData, conn)
  }
}

func (server *Server) sendTable() {
  netData := &NetData{
    Response: NETDATA_UPDATETABLE,
    Table:    server.table,
  }

  server.sendResponseToAll(netData, nil)
}

func (server *Server) removeEliminatedPlayers() {
  netData := &NetData{Response: NETDATA_ELIMINATED}

  for _, player := range server.table.getEliminatedPlayers() {
    conn := server.getPlayerConn(player)
    netData.ID = server.clientIDMap[conn]
    netData.Response = NETDATA_ELIMINATED
    netData.PlayerData = player
    netData.Msg = fmt.Sprintf("<%s id: %s> was eliminated", player.Name, netData.ID[:7])

    server.removePlayer(player)
    server.sendResponseToAll(netData, nil)
  }
}

func (server *Server) roundOver() {
  server.table.finishRound()
  server.sendHands()

  netData := &NetData{
    Response: NETDATA_ROUNDOVER,
    Table:    server.table,
    Msg:      server.table.WinInfo,
  }

  for i, sidePot := range server.table.sidePots.GetAllPots() {
    netData.Msg += fmt.Sprintf("\nsidePot #%d:\n%s", i+1, sidePot.WinInfo)
  }

  server.sendResponseToAll(netData, nil)
  server.sendAllPlayerInfo(false)

  server.removeEliminatedPlayers()

  if server.table.State == TABLESTATE_GAMEOVER {
    server.gameOver()

    return
  }

  server.table.newRound()
  server.table.nextTableAction()
  server.checkBlindsAutoAllIn()
  server.sendDeals()
  server.sendPlayerTurnToAll()
  server.sendTable()
}

func (server *Server) gameOver() {
  fmt.Printf("** game over %s wins **\n", server.table.Winners[0].Name)
  winner := server.table.Winners[0]

  netData := &NetData{
    Response: NETDATA_SERVERMSG,
    Msg:      "game over, " + winner.Name + " wins",
  }

  server.sendResponseToAll(netData, nil)

  server.table.reset(winner) // make a new game while keeping winner connected

  if winnerConn := server.getPlayerConn(winner); winnerConn != server.tableAdmin {
    if winnerConn == nil {
      fmt.Printf("getPlayerConn(): winner (%s) not found\n", winner.Name)
      return
    }
    server.tableAdmin = winnerConn
    sendData(&NetData{Response: NETDATA_MAKEADMIN}, winnerConn)
    server.sendPlayerTurnToAll()

    server.sendResponseToAll(&NetData{
      Response: NETDATA_RESET,
      PlayerData: winner,
      Table: server.table,
    }, nil)
  }
}

func (server *Server) checkBlindsAutoAllIn() {
  if server.table.SmallBlind.Player.Action.Action == NETDATA_ALLIN {
    fmt.Printf("checkBlindsAutoAllIn(): smallblind (%s) forced to go all in\n",
               server.table.SmallBlind.Player.Name)

    if server.table.curPlayer.Player.Name == server.table.SmallBlind.Player.Name {
      // because blind is curPlayer setNextPlayerTurn() will remove the blind
      // from the list for us
      server.table.setNextPlayerTurn()
    } else {
      server.table.curPlayers.RemovePlayer(server.table.SmallBlind.Player)
    }

    server.sendPlayerActionToAll(server.table.SmallBlind.Player, nil)
  }
  if server.table.BigBlind.Player.Action.Action == NETDATA_ALLIN {
    fmt.Printf("checkBlindsAutoAllIn(): bigblind (%s) forced to go all in\n",
               server.table.BigBlind.Player.Name)

    if server.table.curPlayer.Player.Name == server.table.BigBlind.Player.Name {
      // because blind is curPlayer setNextPlayerTurn() will remove the blind
      // from the list for us
      server.table.setNextPlayerTurn()
    } else {
      server.table.curPlayers.RemovePlayer(server.table.BigBlind.Player)
    }

    server.sendPlayerActionToAll(server.table.BigBlind.Player, nil)
  }
}

func (server *Server) postBetting(player *Player, netData *NetData, conn *websocket.Conn) {
  if player != nil {
    server.sendPlayerActionToAll(player, conn)
    server.sendPlayerTurnToAll()
  }

  fmt.Println("postBetting(): done betting...")

  if server.table.bettingIsImpossible() {
    fmt.Println("postBetting(): no more betting possible this round")

    for server.table.State != TABLESTATE_ROUNDOVER {
      server.table.nextCommunityAction()
    }
  } else {
    server.table.nextCommunityAction()
  }

  if server.table.State == TABLESTATE_ROUNDOVER {
    server.roundOver()

    if server.table.State == TABLESTATE_GAMEOVER {
      return // XXX
    }
  } else { // new community card(s)
    netData.Response = server.table.commState2NetDataResponse()
    netData.Table = server.table
    netData.PlayerData = nil

    server.sendResponseToAll(netData, nil)

    server.table.Bet, server.table.better = 0, nil
    for _, player := range server.table.curPlayers.ToPlayerArray() {
      fmt.Printf("clearing %v's action\n", player.Name)
      player.Action.Action = NETDATA_FIRSTACTION
      player.Action.Amount = 0
    }

    server.sendAllPlayerInfo(true)
    server.table.reorderPlayers()
    server.sendPlayerTurnToAll()
    // let players know they should update their current hand after
    // the community action
    server.sendCurHands()
  }
}

func (server *Server) postPlayerAction(player *Player, netData *NetData, conn *websocket.Conn) {
  if server.table.State == TABLESTATE_DONEBETTING {
    server.postBetting(player, netData, conn)
  } else if server.table.State == TABLESTATE_ROUNDOVER {
      // all other players folded before all comm cards were dealt
      // TODO: check for this state in a better fashion
      server.table.finishRound()
      fmt.Printf("winner # %d\n", len(server.table.Winners))
      fmt.Println(server.table.Winners[0].Name + " wins by folds")

      netData.Response = NETDATA_ROUNDOVER
      netData.Table = server.table
      netData.Msg = server.table.Winners[0].Name + " wins by folds"
      netData.PlayerData = nil

      server.sendResponseToAll(netData, nil)

      server.removeEliminatedPlayers()

      if server.table.State == TABLESTATE_GAMEOVER {
        server.gameOver()

        return
      }

      server.table.newRound()
      server.table.nextTableAction()
      server.checkBlindsAutoAllIn()
      server.sendDeals()
      server.sendPlayerTurnToAll()
      server.sendTable()
  } else {
    server.sendPlayerActionToAll(player, conn)
    server.sendPlayerTurnToAll()
  }
}

// cleanly close connections after a server panic()
func (server *Server) serverError(err error) {
  fmt.Println("server panicked")

  for _, conn := range server.clients {
    conn.WriteMessage(websocket.CloseMessage,
      websocket.FormatCloseMessage(websocket.CloseInternalServerErr,
        err.Error()))
  }

  server.errChan <- err
  server.panicked = true
}

type ClientSettings struct {
  Name string
}

func (server *Server) handleClientSettings(conn *websocket.Conn, settings *ClientSettings) error {
  errs := ""

  if conn == nil || settings == nil {
    fmt.Println("handleClientSettings(): called with a nil parameter")

    return errors.New("handleClientSettings(): BUG: called with a nil parameter")
  }

  settings.Name = strings.TrimSpace(settings.Name)
  if settings.Name != "" {
    if len(settings.Name) > 15 {
      fmt.Printf("handleClientSettings(): %p requested a name that was longer " +
                 "than 15 characters. using a default name\n", conn)
      errs += "You've requested a name that was longer than 15 characters. " +
              "Using a default name.\n\n"
      settings.Name = ""
    } else {
      if player := server.playerMap[conn]; player != nil {
        if player.Name == settings.Name {
          fmt.Println("handleClientSettings(): name unchanged")
          errs += "name: unchanged\n\n"
        } else {
          for _, p := range server.table.players {
            if settings.Name == p.Name {
              fmt.Printf("%p requested the name `%s` which is reserved or already taken",
                         conn, settings.Name)
              errs += fmt.Sprintf("Name '%s' already in use. Current name unchanged.\n\n",
                                  settings.Name)
              break
            }
          }
        }
      } else {
        for _, player := range server.table.players {
          if settings.Name == player.Name {
            fmt.Printf("%p requested the name `%s` which is reserved or already taken. " +
                       "using a default name\n", conn, settings.Name)
            errs += fmt.Sprintf("Name '%s' already in use. Using a default name.\n\n",
                                settings.Name)
            settings.Name = ""
            break
          }
        }
      }
    }
  }

  if errs != "" {
    errs = "server response: settings changes: \n\n" + errs
    return errors.New(errs)
  }

  return nil
}

func (server *Server) applyClientSettings(conn *websocket.Conn, settings *ClientSettings) {
  if player := server.playerMap[conn]; player != nil {
    player.setName(settings.Name)
  }
}

func (server *Server) WSCLIClient(w http.ResponseWriter, req *http.Request) {
  if req.Header.Get("keepalive") != "" {
    return // NOTE: for heroku
  }

  conn, err := server.upgrader.Upgrade(w, req, nil)
  if err != nil {
    fmt.Printf("WS upgrade err %s\n", err.Error())

    return
  }

  cleanExit := false
  defer func() {
    if server.panicked { // server panic was already recovered in previous client handler
      return
    }

    if err := recover(); err != nil {
      server.serverError(panicRetToError(err))
    } else { // not a server panic()
      if player := server.playerMap[conn]; player != nil {
        if !cleanExit {
          fmt.Printf("%s had an unclean exit\n", player.Name)
        }
        if server.table.activePlayers.len > 1 &&
           server.table.curPlayer.Player.Name == player.Name {
          server.table.curPlayer.Player.Action.Action = NETDATA_FOLD
          server.table.setNextPlayerTurn()
          server.sendPlayerTurnToAll()
        }
      }

      server.removePlayerByConn(conn)
      server.removeClient(conn)
      server.closeConn(conn)
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

  server.clientIDMap[conn] = randString(20)

  netData := NetData{
    ID:       server.clientIDMap[conn],
    Response: NETDATA_NEWCONN,
    Table:    server.table,
  }

  for {
    _, rawData, err := conn.ReadMessage()
    if err != nil {
      if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
        fmt.Printf("runServer(): readConn() conn: %p err: %v\n", conn, err)
      }

      return
    }

    // we need to set Table member to nil otherwise gob will
    // modify our server.table structure if a user server.sends that member
    netData = NetData{Response: NETDATA_NEWCONN, Table: nil}

    gob.NewDecoder(bufio.NewReader(bytes.NewReader(rawData))).Decode(&netData)

    netData.Table = server.table

    fmt.Printf("recv %s (%d bytes) from %p\n", netDataReqToString(&netData),
               len(rawData), conn)

    if netData.Request == NETDATA_NEWCONN {
      server.clients = append(server.clients, conn)

      server.table.mtx.Lock()
      server.table.NumConnected++
      server.table.mtx.Unlock()

      server.sendResponseToAll(&netData, nil)

      // server.send current player info to this client
      if server.table.NumConnected > 1 {
        netData.Response = NETDATA_CURPLAYERS
        netData.Table = server.table

        for _, player := range server.table.activePlayers.ToPlayerArray() {
          netData.ID = server.clientIDMap[server.getPlayerConn(player)]
          netData.PlayerData = server.table.PublicPlayerInfo(*player)
          sendData(&netData, conn)
        }
      }

      if err := server.handleClientSettings(conn, netData.ClientSettings); err != nil {
        sendData(&NetData{Response: NETDATA_BADREQUEST, Msg: err.Error()}, conn)
      }

      if player := server.table.getOpenSeat(); player != nil {
        server.playerMap[conn] = player

        server.applyClientSettings(conn, netData.ClientSettings)
        fmt.Printf("adding %p as player '%s'\n", &conn, player.Name)

        if server.table.State == TABLESTATE_NOTSTARTED {
          player.Action.Action = NETDATA_FIRSTACTION
          server.table.curPlayers.AddPlayer(player)
        } else {
          player.Action.Action = NETDATA_MIDROUNDADDITION
        }
        server.table.activePlayers.AddPlayer(player)

        if server.table.curPlayer == nil {
          server.table.curPlayer = server.table.curPlayers.node
        }

        if server.table.Dealer == nil {
          server.table.Dealer = server.table.activePlayers.node
        } else if server.table.SmallBlind == nil {
          server.table.SmallBlind = server.table.Dealer.next
        } else if server.table.BigBlind == nil {
          server.table.BigBlind = server.table.SmallBlind.next
        }

        netData.ID = server.clientIDMap[conn]
        netData.Response = NETDATA_NEWPLAYER
        netData.Table = server.table
        netData.PlayerData = server.table.PublicPlayerInfo(*player)

        server.sendResponseToAll(&netData, conn)

        netData.ID = server.clientIDMap[conn]
        netData.Response = NETDATA_YOURPLAYER
        netData.PlayerData = player
        sendData(&netData, conn)
      } else {
        netData.Response = NETDATA_SERVERMSG
        netData.Msg = "No open seats available. You have been added as a spectator"

        sendData(&netData, conn)
      }

      server.sendPlayerTurn(conn)

      if server.tableAdmin == nil {
        server.table.mtx.Lock()
        server.tableAdmin = conn
        server.table.mtx.Unlock()

        sendData(&NetData{Response: NETDATA_MAKEADMIN}, conn)
      }
    } else {
      switch netData.Request {
      case NETDATA_CLIENTEXITED:
        cleanExit = true

        return
      case NETDATA_CLIENTSETTINGS:
        if err := server.handleClientSettings(conn, netData.ClientSettings); err == nil {
          server.applyClientSettings(conn, netData.ClientSettings)

          if player := server.playerMap[conn]; player != nil {
            netData.Response = NETDATA_UPDATEPLAYER
            netData.ID = server.clientIDMap[conn]
            netData.PlayerData = server.table.PublicPlayerInfo(*player)
            netData.Table, netData.Msg = nil, ""

            server.sendResponseToAll(&netData, conn)

            netData.Response = NETDATA_YOURPLAYER
            sendData(&netData, conn)

            netData.Response = NETDATA_SERVERMSG
            netData.Msg = "server updated your settings"
            sendData(&netData, conn)
          }
        } else {
          sendData(&NetData{Response: NETDATA_SERVERMSG, Msg: err.Error()}, conn)
        }
      case NETDATA_STARTGAME:
        if conn != server.tableAdmin {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "only the table admin can do that"
          netData.Table = nil

          sendData(&netData, conn)
        } else if server.table.NumPlayers < 2 {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "not enough players to start"
          netData.Table = nil

          sendData(&netData, conn)
        } else if server.table.State != TABLESTATE_NOTSTARTED {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "this game has already started"
          netData.Table = nil

          sendData(&netData, conn)
        } else { // start game
          server.table.nextTableAction()

          server.sendDeals()
          server.sendAllPlayerInfo(false)
          server.sendPlayerTurnToAll()
          server.sendTable()
        }
      case NETDATA_CHATMSG:
        netData.ID = server.clientIDMap[conn]
        netData.Response = NETDATA_CHATMSG

        if len(netData.Msg) > 256 {
          netData.Msg = netData.Msg[:256] + "(snipped)"
        }

        if player := server.playerMap[conn]; player != nil {
          netData.Msg = fmt.Sprintf("[%s id: %s]: %s", player.Name,
                                    netData.ID[:7], netData.Msg)
        } else {
          netData.Msg = fmt.Sprintf("[spectator id: %s]: %s",
                                    netData.ID[:7], netData.Msg)
        }

        server.sendResponseToAll(&netData, nil)
      case NETDATA_ALLIN, NETDATA_BET, NETDATA_CALL, NETDATA_CHECK, NETDATA_FOLD:
        player := server.playerMap[conn]

        if player == nil {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "you are not a player"
          netData.Table = nil

          sendData(&netData, conn)
          continue
        }

        if server.table.State == TABLESTATE_NOTSTARTED {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "a game has not been started yet"
          netData.Table = nil

          sendData(&netData, conn)
          continue
        }

        if player.Name != server.table.curPlayer.Player.Name {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "it's not your turn"
          netData.Table = nil

          sendData(&netData, conn)
          continue
        }

        if err := server.table.PlayerAction(player, netData.PlayerData.Action); err != nil {
          netData.Response = NETDATA_BADREQUEST
          netData.Table = nil
          netData.Msg = err.Error()

          sendData(&netData, conn)
        } else {
          server.postPlayerAction(player, &netData, conn)
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


func (server *Server) run() error {
  fmt.Printf("starting server on %v\n", server.http.Addr)

  go func() {
    if err := server.http.ListenAndServe(); err != nil {
      fmt.Printf("ListenAndServe(): %s\n", err.Error())
    }
  }()

  select {
  case sig := <-server.sigChan:
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    fmt.Fprintf(os.Stderr, "received signal: %s\n", sig.String())

    // TODO: ignore irrelevant signals
    server.sendResponseToAll(&NetData{Response: NETDATA_SERVERCLOSED}, nil)

    if err := server.http.Shutdown(ctx); err != nil {
      fmt.Fprintf(os.Stderr, "server.Shutdown(): %s\n", err.Error())
      return err
    }

    return nil
  case err := <-server.errChan:
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    fmt.Fprintf(os.Stderr, "irrecoverable server error: %s\n", err.Error())

    if err := server.http.Shutdown(ctx); err != nil {
      fmt.Fprintf(os.Stderr, "server.Shutdown(): %s\n", err.Error())
      return err
    }

    return err
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

    sendData(&NetData{Request: NETDATA_NEWCONN,
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

        cli.go:
        - figure out why refocusing on a primitive increments the highlighted
          sub element
        - add orange border to new betters
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
