package main

import (
	"fmt"

	//"net"

	//"io"

	"errors"
	"sort"
	"sync"

	//_ "net/http/pprof"

	math_rand "math/rand"
)

// ranks
const (
  RankMuck = iota - 1
  RankHighCard
  RankPair
  RankTwoPair
  RankTrips
  RankStraight
  RankFlush
  RankFullHouse
  RankQuads
  RankStraightFlush
  RankRoyalFlush
)

// cards
const (
  CardAceLow = iota + 1
  CardTwo
  CardThree
  CardFour
  CardFive
  CardSix
  CardSeven
  CardEight
  CardNine
  CardTen
  CardJack
  CardQueen
  CardKing
  CardAce
)

// suits
const (
  SuitClub = iota
  SuitDiamond
  SuitHeart
  SuitSpade
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
    RankMuck:          "muck",
    RankHighCard:      "high card",
    RankPair:          "pair",
    RankTwoPair:       "two pair",
    RankTrips:         "three of a kind",
    RankStraight:      "straight",
    RankFlush:         "flush",
    RankFullHouse:     "full house",
    RankQuads:         "four of a kind",
    RankStraightFlush: "straight flush",
  }

  if rankName, ok := rankNameMap[hand.Rank]; ok {
    return rankName
  }

  panic("Hand.RankName(): BUG")
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
  return !p.IsVacant && p.Action.Action != NetDataMidroundAddition &&
         p.Action.Action != NetDataFold && p.Action.Action != NetDataAllIn
}

type PlayerNode struct {
  /*prev,*/ next *PlayerNode // XXX don't think i need this to be a pointer. check back
  Player *Player
}

// circular list of players at the poker table
type playerList struct {
  len int
  name string
  head *PlayerNode // XXX don't think i need this to be a pointer. check back
}

func (list *playerList) Init(name string, players []*Player) error {
  if len(players) < 2 {
    return errors.New("playerList.Init(): players param must be >= 2")
  }

  list.name = name

  list.head = &PlayerNode{Player: players[0]}
  head := list.head
  for _, p := range players[1:] {
    list.head.next = &PlayerNode{Player: p}
    list.head = list.head.next
  }
  list.head.next = head
  list.head = head
  list.len = len(players)

  return nil
}

func (list *playerList) Print() {
  fmt.Printf("<%s> len==%v [ ", list.name, list.len)
  for i, n := 0, list.head; i < list.len; i++ {
    fmt.Printf("%s n=> %s ", n.Player.Name, n.next.Player.Name)
    n=n.next
    if i == list.len-1 {
      fmt.Printf("| n=> %s ", n.next.Player.Name)
    }
  }
  fmt.Println("]")
}

func (list *playerList) Clone(newName string) playerList {
  if list.len == 0 {
    return playerList{}
  }

  if list.len == 1 {
    clonedList := playerList{name: newName,
                             head: &PlayerNode{Player: list.head.Player}, len: 1}
    clonedList.head.next = clonedList.head

    return clonedList
  }

  clonedList := playerList{}
  clonedList.Init(newName, list.ToPlayerArray())

  return clonedList
}

func (list *playerList) AddPlayer(player *Player) {
  if list.len == 0 {
    list.head = &PlayerNode{Player: player}
    list.head.next = list.head
  } else {
    newNode := &PlayerNode{Player: player, next: list.head}

    node := list.head
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

  fmt.Printf("playerList.RemovePlayer(): <%s> called for %s\n", list.name, player.Name)
  fmt.Printf("playerList.RemovePlayer(): was ") ; list.Print()

  foundPlayer := true

  defer func() {
    if foundPlayer {
      list.len--
    }
    fmt.Printf(" now ") ; list.Print()
  }()

  node, prevNode := list.head, list.head
  for i := 0; i < list.len; i++ {
    if node.Player.Name == player.Name {
      if i == 0 {
        if list.len == 1 {
          list.head = nil
          return nil
        }

        list.head = list.head.next

        tailNode := list.head
        for j := 0; j < list.len-2; j++ {
          tailNode = tailNode.next
        }
        tailNode.next = list.head

        return list.head
      } else {
        prevNode.next = node.next

        return prevNode.next
      }
    }

    prevNode = node
    node = node.next
  }

  fmt.Printf("playerList.RemovePlayer(): %s not found in list\n", player.Name)

  foundPlayer = false
  return nil // player not found
}

func (list *playerList) GetPlayerNode(player *Player) *PlayerNode {
  node := list.head

  //fmt.Printf("playerList.GetPlayerNode(): called for %s\n", player.Name)
  //list.ToNodeArray()

  for i := 0; i < list.len; i++ {
    if node.Player.Name == player.Name {
      return node
    }
    node = node.next
  }

  return nil
}

func (list *playerList) SetHead(node *PlayerNode) {
  if node == nil {
    fmt.Printf("%s.SetHead(): setting parameter is nil")
    if list.len != 0 {
      fmt.Printf(" with a nonempty list\n")
    } else {
      fmt.Println()
    }
  }

  list.head = node
}

func (list *playerList) ToNodeArray() []*PlayerNode {
  nodes := make([]*PlayerNode, 0)

  for i, node := 0, list.head; i < list.len; i++ {
    nodes = append(nodes, node)
    node = node.next
  }

  //fmt.Printf("playerList.ToNodeArray(): ") ; list.Print()

  return nodes
}

func (list *playerList) ToPlayerArray() []*Player {
  if list.len == 0 {
    return nil
  }

  players := make([]*Player, 0)

  for i, node := 0, list.head; i < list.len; i++ {
    players = append(players, node.Player)
    node = node.next
  }

  //fmt.Printf("playerList.ToPlayerArray(): ") ; list.Print()

  return players
}

func (player *Player) Init(name string, isCPU bool) error {
  player.defaultName = name
  player.Name = name
  player.IsCPU = isCPU

  player.IsVacant = true

  player.ChipCount = 1e5 // XXX
  player.NewCards()

  player.Action = Action{Action: NetDataVacantSeat}

  return nil
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
  player.Action.Action = NetDataVacantSeat
}

func (player *Player) ChipCountToString() string {
  return printer.Sprintf("%d", player.ChipCount)
}

func (player *Player) ActionToString() string {
  switch player.Action.Action {
  case NetDataAllIn:
    return printer.Sprintf("all in (%d chips)", player.Action.Amount)
  case NetDataBet:
    return printer.Sprintf("raise (bet %d chips)", player.Action.Amount)
  case NetDataCall:
    return printer.Sprintf("call (%d chips)", player.Action.Amount)
  case NetDataCheck:
    return "check"
  case NetDataFold:
    return "fold"

  case NetDataVacantSeat:
    return "seat is open" // XXX
  case NetDataPlayerTurn:
    return "(player's turn) waiting for action"
  case NetDataFirstAction:
    return "waiting for first action"
  case NetDataMidroundAddition:
    return "waiting to add to next round"

  default:
    return "bad player state"
  }
}

type Deck struct {
  pos   uint
  cards Cards
  size  int
}

func (deck *Deck) Init() error {
  deck.size = 52 // 52 cards in a poker deck
  deck.cards = make(Cards, deck.size, deck.size)

  for suit := SuitClub; suit <= SuitSpade; suit++ {
    for c_num := CardTwo; c_num <= CardAce; c_num++ {
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
    for i := 0; i < deck.size; i++ {
      randIdx := math_rand.Intn(deck.size)
      // swap
      deck.cards[randIdx], deck.cards[i] = deck.cards[i], deck.cards[randIdx]
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
  TableStateNotStarted TableState = iota

  TableStatePreFlop
  TableStateFlop
  TableStateTurn
  TableStateRiver

  TableStateRounds
  TableStatePlayerRaised
  TableStateDoneBetting

  TableStateShowHands
  TableStateSplitPot
  TableStateRoundOver
  TableStateNewRound
  TableStateGameOver
  TableStateReset
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
    printer.Printf("Pot.RemovePlayer(): clearing %v bet sidePot\n", pot.Bet)
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
    panic(fmt.Sprintf("SidePotArray.Insert(): invalid index '%v'\n", idx))
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
    fmt.Printf("SidePotArray.CloseAll(): closing sidePot %d\n", i)
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

const (
  TableLockNone = iota
  TableLockPlayers
  TableLockSpectators
  TableLockAll
)
type TableLock int

var TableLockNameMap = map[TableLock]string{
  TableLockNone:       "no lock",
  TableLockPlayers:    "player lock",
  TableLockSpectators: "spectator lock",
  TableLockAll:        "player & spectator lock",
}

func TableLockToString(lock TableLock) string {
  return TableLockNameMap[lock]
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
  Lock         TableLock  // table admin option that restricts new connections

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

  fmt.Println("Table.reset(): resetting table")

  table.Ante = 10

  table.newCommunity()

  for _, node := range table.curPlayers.ToNodeArray() {
    if player == nil || player.Name != node.Player.Name {
      fmt.Printf("Table.reset(): cleared %s\n", node.Player.Name)
      node.Player.Clear()
      table.curPlayers.RemovePlayer(node.Player)
    } else {
      fmt.Printf("Table.reset(): skipped %s\n", node.Player.Name)
      table.curPlayers.SetHead(node)

      player.NewCards()
      player.Action.Action, player.Action.Amount = NetDataFirstAction, 0
    }
  }

  table.Winners, table.better = nil, nil

  if table.curPlayers.len == 0 && player != nil {
    fmt.Printf("Table.reset(): curPlayers was empty, adding winner %s\n", player.Name)
    table.curPlayers.AddPlayer(player)
  } else {
    table.curPlayer = table.curPlayers.head
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

  table.State = TableStateNotStarted
  table.Lock = TableLockNone

  table.Dealer = table.activePlayers.head

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

func (table *Table) InBettingState() bool {
  if table.State == TableStateNotStarted ||
     table.State == TableStateDoneBetting ||
     table.State == TableStateRoundOver ||
     table.State == TableStateShowHands ||
     table.State == TableStateSplitPot ||
     table.State == TableStateNewRound ||
     table.State == TableStateGameOver {
    return false
  }

  return true
}

func (table *Table) TableStateToString() string {
  tableStateNameMap := map[TableState]string{
    TableStateNotStarted: "waiting for start",

    TableStatePreFlop: "preflop",
    TableStateFlop:    "flop",
    TableStateTurn:    "turn",
    TableStateRiver:   "river",

    TableStateRounds:    "betting rounds",
    TableStateRoundOver: "round over",
    TableStateNewRound:  "new round",
    TableStateGameOver:  "game over",

    TableStatePlayerRaised: "player raised",
    TableStateDoneBetting:  "finished betting",
    TableStateShowHands:    "showing hands",
    TableStateSplitPot:     "split pot",
  }

  if state, ok := tableStateNameMap[table.State]; ok {
    return state
  }

  return "BUG: bad table state"
}

func (table *Table) commState2NetDataResponse() int {
  commStateNetDataMap := map[TableState]int{
    TableStateFlop:  NetDataFlop,
    TableStateTurn:  NetDataTurn,
    TableStateRiver: NetDataRiver,
  }

  if netDataResponse, ok := commStateNetDataMap[table.CommState]; ok {
    return netDataResponse
  }

  fmt.Printf("Table.commState2NetDataResponse(): bad state `%v`\n", table.CommState)
  return NetDataBadRequest
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

func (table *Table) TableLockToString() string {
  return TableLockToString(table.Lock)
}

func (table *Table) PublicPlayerInfo(player Player) *Player {
  if table.State != TableStateShowHands {
    player.Hole, player.Hand = nil, nil
  }

  return &player
}

func (table *Table) allInCount() int {
  allIns := 0

  for _, p := range table.activePlayers.ToPlayerArray() {
    fmt.Printf("Table.allInCount(): <%s> action: %v\n", p.Name, p.ActionToString())
    if p.Action.Action == NetDataAllIn {
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
    fmt.Printf("Table.closeSidePots(): closing mainpot\n")
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
    panic("BUG: Table.getChipLeaders() called with < 2 non-folded/allin players")
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
      seat.Action.Action != NetDataMidroundAddition {
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
    if player.Action.Action == NetDataMidroundAddition {
      fmt.Printf("Table.addNewPlayers(): adding new player: %s\n", player.Name)
      player.Action.Action = NetDataFirstAction
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
    table.State = TableStateGameOver
  } else {
    fmt.Printf("Table.getEliminatedPlayers(): len ret: %v NP-1: %v\n",
               uint(len(ret)), table.NumPlayers-1)
  }

  fmt.Printf("Table.getEliminatedPlayers(): [")
  for _, p := range ret {
    fmt.Printf(" %s ", p.Name)
  }; fmt.Println("]")

  return ret
}

// resets the active players list head to
// Bb+1 pre-flop
// Sb post-flop
func (table *Table) reorderPlayers() {
  if table.State == TableStateNewRound ||
    table.State == TableStatePreFlop {
    table.activePlayers.SetHead(table.BigBlind.next)
    table.curPlayers.SetHead(table.curPlayers.GetPlayerNode(table.BigBlind.next.Player))
    assert(table.curPlayers.head != nil,
           "Table.reorderPlayers(): couldn't find Bb+1 player node")
    fmt.Printf("Table.reorderPlayers(): curPlayers head now: %s\n",
               table.curPlayers.head.Player.Name)
  } else { // post-flop
    smallBlindNode := table.SmallBlind
    if smallBlindNode == nil { // smallblind left mid game
      if table.Dealer != nil {
        smallBlindNode = table.Dealer.next
      } else if table.BigBlind != nil {
        smallBlindNode = table.activePlayers.head
        // definitely considering doubly linked lists now *sigh*
        for smallBlindNode.next.Player.Name != table.BigBlind.Player.Name {
          smallBlindNode = smallBlindNode.next
        }
      } else {
        fmt.Println("Table.reorderPlayers(): dealer, Sb & Bb all left mid round")
        table.handleOrphanedSeats()
        smallBlindNode = table.SmallBlind
      }
      fmt.Printf("Table.reorderPlayers(): smallblind left mid round, setting curPlayer to %s\n",
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

      assert(smallBlindNode != nil, "Table.reorderPlayers(): couldn't find a nonfolded player after Sb")

      fmt.Printf("smallBlind (%s) not active, setting curPlayer to %s\n", table.SmallBlind.Player.Name,
                 smallBlindNode.Player.Name)
    }
    table.curPlayers.SetHead(smallBlindNode) // small blind (or next active player)
                                             // is always first better after pre-flop
  }

  table.curPlayer = table.curPlayers.head
}


func (table *Table) handleOrphanedSeats() {
  // TODO: this is the corner case where D, Sb & Bb all leave mid-game. need to
  //       find a way to keep track of dealer pos to rotate properly.
  //
  //       considering making lists doubly linked.
  if table.Dealer == nil && table.SmallBlind == nil && table.BigBlind == nil {
    fmt.Println("Table.handleOrphanedSeats(): D, Sb & Bb all nil, resetting to activePlayers head")
    table.Dealer = table.activePlayers.head
    table.SmallBlind = table.Dealer.next
    table.BigBlind = table.SmallBlind.next
  }
  if table.Dealer == nil && table.SmallBlind == nil && table.BigBlind != nil {
    var newDealerNode *PlayerNode
    for i, n := 0, table.activePlayers.head; i < table.activePlayers.len; i++ {
      if n.next.next.Player.Name == table.BigBlind.Player.Name {
        newDealerNode = n
        break
      }
      n = n.next
    }

    assert(newDealerNode != nil, "Table.handleOrphanedSeats(): newDealerNode == nil")

    fmt.Printf("Table.handleOrphanedSeats(): setting dealer to %s\n", newDealerNode.Player.Name)

    table.Dealer = newDealerNode
    table.SmallBlind = table.Dealer.next
    table.BigBlind = table.SmallBlind.next
  }
  if table.Dealer == nil {
    var newDealerNode *PlayerNode
    for i, n := 0, table.activePlayers.head; i < table.activePlayers.len; i++ {
      if n.next.Player.Name == table.SmallBlind.Player.Name {
        newDealerNode = n
        break
      }
      n = n.next
    }

    assert(newDealerNode != nil, "Table.handleOrphanedSeats(): newDealerNode == nil")

    fmt.Printf("Table.handleOrphanedSeats(): setting dealer to %s\n", newDealerNode.Player.Name)

    table.Dealer = newDealerNode
  } else if table.SmallBlind == nil {
    table.SmallBlind = table.Dealer.next
    fmt.Printf("Table.handleOrphanedSeats(): setting smallblind to %s\n", table.SmallBlind.Player.Name)
  } else if table.BigBlind == nil {
    table.BigBlind = table.SmallBlind.next
    fmt.Printf("Table.handleOrphanedSeats(): setting bigblind to %s\n", table.BigBlind.Player.Name)
  }
}

// rotates the dealer and blinds
func (table *Table) rotatePlayers() {
  if table.State == TableStateNotStarted || table.activePlayers.len < 2 {
    return
  }

  if table.Dealer == nil || table.SmallBlind == nil || table.BigBlind == nil {
    table.handleOrphanedSeats()
  }

  fmt.Printf("Table.rotatePlayers(): D=%s S=%s B=%s => ",
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
  fmt.Printf("Table.setNextPlayerTurn(): curPlayer: %s\n", table.curPlayer.Player.Name)
  if table.State == TableStateNotStarted {
    return
  }

  table.mtx.Lock()
  defer table.mtx.Unlock()

  Panic := &Panic{}
  Panic.Init()

  thisPlayer := table.curPlayer // save in case we need to remove from curPlayers list

  defer Panic.ifNoPanic(func() {
    if table.State == TableStateDoneBetting {
      table.better = nil
      table.closeSidePots()
    }

    if thisPlayer.Player.Action.Action == NetDataAllIn {
      nextNode := table.curPlayers.RemovePlayer(thisPlayer.Player)
      if nextNode != nil {
        fmt.Printf("Table.setNextPlayerTurn(): removing %s, curPlayers head is %s\n",
                   thisPlayer.Player.Name, table.curPlayers.head.Player.Name)
        table.curPlayer = nextNode
        fmt.Printf("  now %s\n", table.curPlayers.head.Player.Name)
      }
    }

    fmt.Printf("Table.setNextPlayerTurn(): new curPlayer: %v\n",
               table.curPlayer.Player.Name)
    table.curPlayers.ToPlayerArray()
  })

  if table.curPlayers.len == 1 {
    fmt.Println("Table.setNextPlayerTurn(): table.curPlayers.len == 1")
    if table.allInCount() == 0 { // win by folds
      fmt.Println("Table.setNextPlayerTurn(): allInCount == 0")
      table.State = TableStateRoundOver // XXX
    } else {
      table.State = TableStateDoneBetting
    }

    return
  }

  if thisPlayer.Player.Action.Action == NetDataFold {
    nextNode := table.curPlayers.RemovePlayer(thisPlayer.Player)
    if nextNode != nil {
      fmt.Printf("Table.setNextPlayerTurn(): removing %s, curPlayers head is %s\n",
                 thisPlayer.Player.Name, table.curPlayers.head.Player.Name)
      table.curPlayer = nextNode
      fmt.Printf("  now %s\n", table.curPlayers.head.Player.Name)
    }
  } else {
    table.curPlayer = thisPlayer.next
  }

  if table.curPlayers.len == 1 && table.allInCount() == 0 {
    fmt.Println("Table.setNextPlayerTurn(): table.curPlayers.len == 1 with allInCount of 0 after fold")
    table.State = TableStateRoundOver // XXX

    return
  } else if thisPlayer.next == table.curPlayers.head &&
            table.curPlayers.head.Player.Action.Action != NetDataFirstAction &&
            thisPlayer.Player.Action.Action != NetDataBet {
    // NOTE: curPlayers head gets shifted with allins / folds so we check for
    //       firstaction
    fmt.Printf("Table.setNextPlayerTurn(): last player (%s) didn't raise\n",
               thisPlayer.Player.Name)
    fmt.Printf(" curPlayers == %s curPlayers.next == %s\n",
               table.curPlayers.head.Player.Name, table.curPlayer.next.Player.Name)

    table.State = TableStateDoneBetting
  } else {;
    //table.curPlayer = table.curPlayer.next
  }
}

func (table *Table) PlayerAction(player *Player, action Action) error {
  if table.State == TableStateNotStarted {
    return errors.New("game has not started yet")
  }

  if table.State != TableStateRounds &&
    table.State != TableStatePlayerRaised &&
    table.State != TableStatePreFlop {
    // XXX
    return errors.New("invalid table state: " + table.TableStateToString())
  }

  var blindRequiredBet uint = 0

  isSmallBlindPreFlop := false

  if table.CommState == TableStatePreFlop &&
     table.State != TableStatePlayerRaised { // XXX mixed states...
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
        fmt.Printf("Table.PlayerAction(): handleSidePots(): firstSidePot: <%s> allin " +
                   "created an empty betting sidepot\n",
                   player.Name)
      } else {
        // get players who already called the last bet,
        // sub the delta of the last bet and this players
        // chipcount in mainpot, then add them to the mainpot & sidepot.
        for playerNode := table.curPlayers.head;
            playerNode.Player.Name != player.Name;
            playerNode = playerNode.next {
          p := playerNode.Player
          if p.Name == player.Name {
            break
          }

          table.MainPot.Total -= sidePot.Bet

          printer.Printf("Table.PlayerAction(): handleSidePots(): firstSidePot: <%s> " +
                         "sub %d from mainpot, add same amt to sidePot\n", p.Name, sidePot.Bet)

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

    if player.Action.Action == NetDataAllIn {
      if !table.MainPot.IsClosed {
        if table.MainPot.Bet > player.Action.Amount {
          printer.Printf("Table.PlayerAction(): handleSidePots(): <%s> moving previous " +
                         "mainpot to first sidepot with bet of %v\n", player.Name,
                         table.MainPot.Bet)
          sidePot := &SidePot{
            Pot{
              Bet: table.MainPot.Bet,
            },
          }
          betDiff := table.MainPot.Bet - player.Action.Amount
          printer.Printf("Table.PlayerAction(): handleSidePots(): <%s> changed mainpot " +
                         "bet from %v to %v betDiff %v\n",
                         player.Name, table.MainPot.Bet, player.Action.Amount, betDiff)

          table.MainPot.Bet = player.Action.Amount
          table.MainPot.Total -= betDiff * uint(len(table.MainPot.Players))

          sidePot.Init(table.MainPot.Players, nil)
          table.sidePots.AllInPots.Insert(sidePot, 0)
          table.MainPot.AddPlayer(player)
        } else if table.MainPot.Bet == player.Action.Amount {
          printer.Printf("Table.PlayerAction(): handleSidePots(): <%s> allin (%v) " +
                         "matched the mainpot allin\n",
                         player.Name, player.Action.Amount)

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
            fmt.Printf("Table.PlayerAction(): handleSidePots(): <%s> all in matched a " +
                       "previous all in sidePot\n", player.Name)
          case -1:
            betDiff := table.sidePots.BettingPot.Bet - player.Action.Amount

            printer.Printf("Table.PlayerAction(): handleSidePots(): <%s> %v all in is " +
                           "largest AllIn sidePot\n",
                           player.Name, player.ChipCountToString())
            printer.Printf(" bettingpot bet changed from %v to %v\n",
                           table.sidePots.BettingPot.Bet,
                           table.sidePots.BettingPot.Bet - player.Action.Amount)
            printer.Printf(" bettingpot pot changed from %v to %v\n",
                           table.sidePots.BettingPot.Total,
                           table.sidePots.BettingPot.Bet - (betDiff * uint(len(table.sidePots.BettingPot.Players))))

            if !table.MainPot.HasPlayer(player) {
              fmt.Printf(" <%s> adding to mainpot\n", player.Name)
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
              fmt.Printf("Table.PlayerAction(): handleSidePots(): <%s> " +
                         "adding to mainpot\n", player.Name)
              table.MainPot.AddPlayer(player)
              table.MainPot.Total += table.MainPot.Bet
            }

            printer.Printf(" <%s> inserting %v allin at idx %v\n", player.Name,
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
          fmt.Printf("Table.PlayerAction(): handleSidePots(): mpclosed: <%s> all in " +
                     "matched a previous all in sidePot\n", player.Name)
        case -1:
          betDiff := table.sidePots.BettingPot.Bet - player.Action.Amount
          if betDiff == 0 {
            betDiff = player.Action.Amount
          }

          printer.Printf("Table.PlayerAction(): handleSidePots(): <%s> %v all in is " +
                         "largest AllIn sidePot\n",
                        player.Name, player.ChipCountToString())
          printer.Printf(" bettingpot bet changed from %v to %v\n",
                         table.sidePots.BettingPot.Bet,
                         table.sidePots.BettingPot.Bet - player.Action.Amount)
          printer.Printf(" bettingpot pot changed from %v to %v\n",
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
          printer.Printf("Table.PlayerAction(): handleSidePots(): mpclosed: <%s> inserting " +
                         "%v allin at idx %v\n",
                         player.Name, player.ChipCountToString(), idx)
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
        printer.Sprintf("Table.PlayerAction(): handleSidePots(): <%v> cc: %v cant match mainpot bet %v",
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
          printer.Printf(" adding %s to open sidePot (%v bet)\n", player.Name, sidePot.Bet)
        } else {
          printer.Printf(" %s already in sidePot (%v bet)\n", player.Name, sidePot.Bet)
        }
      }

      bettingPot := table.sidePots.BettingPot
      switch player.Action.Action {
      case NetDataBet:
        lspb := table.sidePots.BettingPot.Bet
        if table.State == TableStatePlayerRaised &&
          player.Action.Amount > bettingPot.Bet {
          fmt.Printf(" bettingpot: %s re-raised\n", player.Name)
          if bettingPot.Bet == 0 {
            fmt.Printf(" bettingpot: %s re-raised an all-in\n", player.Name)
          }
          bettingPot.Total += player.Action.Amount - bettingPot.Bet
          bettingPot.Bet = player.Action.Amount
        } else {
          fmt.Printf(" bettingpot: %s made new bet\n", player.Name)
          bettingPot.Bet = player.Action.Amount
          bettingPot.Total += bettingPot.Bet
        }
        if bettingPot.Bet != lspb {
          printer.Printf(" bettingpot: %s changed betting pot bet from %d to %d\n", player.Name,
            lspb, player.Action.Amount)
        }
      case NetDataCall:
        fmt.Printf(" bettingpot: %s called\n", player.Name)
        bettingPot.Total += bettingPot.Bet
        bettingPot.AddPlayer(player)
      }
    }
  }

  if table.curPlayers.len == 1 &&
     (action.Action == NetDataAllIn || action.Action == NetDataBet) {
    return errors.New(printer.Sprintf("you must call the raise (%d chips) or fold", table.Bet))
  }

  if player.ChipCount == 0 && action.Action != NetDataAllIn {
    fmt.Printf("Table.PlayerAction(): changing %s's all-in bet to an allin action\n", player.Name)
    action.Action = NetDataAllIn
  }

  switch action.Action {
  case NetDataAllIn:
    player.Action.Action = NetDataAllIn

    // we need to add the blind's chips back, otherwise it would get added to current bet
    //player.Action.Amount -= blindRequiredBet
    //player.ChipCount += blindRequiredBet

    prevChips := player.Action.Amount
    printer.Printf("Table.PlayerAction(): ALLIN: %s prevChips == %v\n", player.Name, prevChips)

    player.Action.Amount += prevChips
    player.ChipCount += prevChips

    if table.bettingIsImpossible() {
      fmt.Printf("Table.PlayerAction(): last player (%s) went all-in\n", player.Name)
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
        table.State = TableStatePlayerRaised
        table.better = player
        fmt.Printf("Table.PlayerAction(): setting curPlayers head to %s\n",
                   table.curPlayer.Player.Name)
        table.curPlayers.SetHead(table.curPlayer) // NOTE: the new better always
                                                  // becomes the head of the table
      }
    }

    handleSidePots()

    player.ChipCount -= player.Action.Amount

    //table.curPlayers.RemovePlayer(player)
  case NetDataBet:
    prevChips := player.Action.Amount
    printer.Printf("Table.PlayerAction(): BET: %s prevChips == %v\n", player.Name, prevChips)

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

    printer.Printf("Table.PlayerAction(): BET: %s adding prevChips %v\n", player.Name, prevChips)
    player.ChipCount += prevChips

    // NOTE: A chipleader can only bet what at least one other player can match.
    if player.ChipCount == chipLeaderCount {
      player.Action.Amount = minUInt(action.Amount, secondChipLeaderCount)
    } else {
      player.Action.Amount = action.Amount
    }

    if action.Amount == player.ChipCount {
      player.Action.Action = NetDataAllIn
    } else {
      player.Action.Action = NetDataBet
    }

    if player.Action.Action == NetDataAllIn || !table.sidePots.IsEmpty() {
      handleSidePots()
    } else {
      table.MainPot.Total += player.Action.Amount - prevChips
    }

    player.ChipCount -= player.Action.Amount

    table.Bet = player.Action.Amount

    fmt.Printf("setting curPlayers head to %s\n", table.curPlayer.Player.Name)
    table.curPlayers.SetHead(table.curPlayer)
    table.better = player
    table.State = TableStatePlayerRaised
  case NetDataCall:
    if table.State != TableStatePlayerRaised && !isSmallBlindPreFlop {
      return errors.New("nothing to call")
    }

    if (table.SmallBlind != nil && player.Name == table.SmallBlind.Player.Name) ||
       (table.BigBlind != nil && player.Name == table.BigBlind.Player.Name) {
      fmt.Printf("Table.PlayerAction(): CALL: %s action.amt: %v\n", player.Name, player.Action.Amount)
    }

    // XXX we need to add the blind's chips back, otherwise it would get added to current bet
    // NOTE: Amount is always >= blindRequiredBet
    /*player.Action.Amount -= blindRequiredBet
    player.ChipCount += blindRequiredBet
    table.MainPot.Total -= blindRequiredBet*/

    prevChips := player.Action.Amount - blindRequiredBet
    printer.Printf("Table.PlayerAction(): CALL: %s prevChips == %v\n", player.Name, prevChips)

    //if blindRequiredBet != 0 && table.Bet > table.Ante {
    //  player.Action.Amount -= blindRequiredBet
    //}

    player.ChipCount += prevChips

    // delta of bet & curPlayer's last bet
    betDiff := table.Bet - player.Action.Amount

    fmt.Printf("Table.PlayerAction(): %s betDiff: %v\n", player.Name, betDiff)

    if table.Bet >= player.ChipCount {
      player.Action.Action = NetDataAllIn
      player.Action.Amount = player.ChipCount

      handleSidePots()

      player.ChipCount = 0
    } else {
      player.Action.Action = NetDataCall
      player.Action.Amount = table.Bet - blindRequiredBet

      if !table.sidePots.IsEmpty() {
        handleSidePots()
      } else {
        table.MainPot.Total += betDiff
      }
      player.ChipCount -= player.Action.Amount
    }
  case NetDataCheck:
    if table.State == TableStatePlayerRaised {
      return errors.New(printer.Sprintf("you must call the raise (%d chips)", table.Bet))
    }

    if isSmallBlindPreFlop {
      return errors.New(printer.Sprintf("you must call the big blind (+%d chips)", blindRequiredBet))
    }

    if player.ChipCount == 0 { // big blind had a chipcount <= ante
      player.Action.Action = NetDataAllIn
    } else {
      player.Action.Action = NetDataCheck
    }
  case NetDataFold:
    player.Action.Action = NetDataFold

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

  table.State = TableStatePreFlop
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
  case TableStatePreFlop:
    table.DoFlop()

    table.CommState = TableStateFlop
    if !table.bettingIsImpossible() { // else all players went all in preflop
                                      // and we are in the all-in loop
      table.reorderPlayers()
    }
  case TableStateFlop:
    table.DoTurn()

    table.CommState = TableStateTurn
  case TableStateTurn:
    table.DoRiver()

    table.CommState = TableStateRiver
  case TableStateRiver:
    table.State = TableStateRoundOver // XXX shouldn't mix these states

    return
  default:
    panic("BUG: Table.nextCommunityAction(): invalid community state")
  }

  table.State = TableStateRounds
}

func (table *Table) nextTableAction() {
  switch table.State {
  case TableStateNotStarted:
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

    table.CommState = TableStatePreFlop

    table.reorderPlayers() // NOTE: need to call this to properly set curPlayer
  case TableStateNewRound:
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
      table.SmallBlind.Player.Action.Action = NetDataAllIn
    }
    if table.BigBlind.Player.ChipCount == 0 {
      table.BigBlind.Player.Action.Action = NetDataAllIn
    }

    table.MainPot.Total = table.SmallBlind.Player.Action.Amount + table.BigBlind.Player.Action.Amount

    table.Deal()

    table.CommState = TableStatePreFlop
  case TableStateGameOver:
    fmt.Printf("Table.nextTableAction(): game over!\n")

  default:
    fmt.Printf("Table.nextTableAction(): BUG: called with improper state (" +
      table.TableStateToString() + ")")
  }
}

func (table *Table) DoFlop() {
  for i := 0; i < 3; i++ {
    table.AddToCommunity(table.deck.Pop())
  }
  table.PrintCommunity()
  table.SortCommunity()

  table.State = TableStateRounds
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
    if player.Action.Action != NetDataFold {
      players = append(players, player)
    }
  }

  assert(len(players) != 0, "Table.getNonFoldedPlayers(): BUG: len(players) == 0")

  return players
}

func (table *Table) newRound() {
  table.deck.Shuffle()

  table.addNewPlayers()

  for _, player := range table.activePlayers.ToPlayerArray() {
    player.NewCards()

    player.Action.Amount = 0
    player.Action.Action = NetDataFirstAction // NOTE: set twice w/ new player
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
  table.State = TableStateNewRound
}

func (table *Table) finishRound() {
  table.mtx.Lock()
  defer table.mtx.Unlock()
  // special case for when everyone except a folded player
  // leaves the table
  if table.activePlayers.len == 1 &&
     table.activePlayers.head.Player.Action.Action == NetDataFold {
    fmt.Printf("Table.finishRound(): only one folded player (%s) left at table. " +
               "abandoning all pots\n",
               table.activePlayers.head.Player.Name)

    table.State = TableStateGameOver
    table.Winners = []*Player{table.activePlayers.head.Player}

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
      printer.Sprintf("BUG: Table.finishRound(): %s won by folds but there are sidepots", player.Name))

    table.State = TableStateRoundOver
    table.Winners = players

    return
  }

  table.State = TableStateShowHands

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

    table.State = TableStateSplitPot
  }

  table.Winners = bestPlayers

  for i, sidePot := range table.sidePots.GetAllPots() {
    // remove players that folded from sidePots
    // XXX: probably not the best place to do this.
    for _, player := range sidePot.Players {
      if player.Action.Action == NetDataFold {
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

        //table.State = TableStateSplitPot
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

  if table.State == TableStatePreFlop ||
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
        player.Hand.Rank = RankFlush

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

      if (*cards)[0].NumValue != CardTwo {
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
      if (*cards)[high].NumValue == CardAce {
        player.Hand.Rank = RankRoyalFlush
      } else {
        player.Hand.Rank = RankStraightFlush
      }
    } else {
      player.Hand.Rank = RankStraight
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

    if player.Hand.Rank == RankRoyalFlush ||
      player.Hand.Rank == RankStraightFlush {
      return
    }

    if isFlush, _ := gotFlush(cards, player, true); isFlush {
      return
    }

    // check for A to 5
    if !isStraight && cards[len(cards)-1].NumValue == CardAce {
      gotStraight(&cards, player, 3, true)
    }

    if player.Hand.Rank == RankStraight {
      return
    }

    // muck
    player.Hand.Rank = RankHighCard
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

    player.Hand.Rank = RankQuads
    player.Hand.Cards = append(Cards{kicker}, cards[quadsIdx:quadsIdx+4]...)

    return
  }

  // fullhouse search //
  //
  // NOTE: we check for a fullhouse before a straight flush because it's
  // impossible to have both at the same time and searching for the fullhouse
  // first saves some cycles+space
  if matchHands.trips != nil && matchHands.pairs != nil {
    player.Hand.Rank = RankFullHouse

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

    if player.Hand.Rank == RankRoyalFlush ||
      player.Hand.Rank == RankStraightFlush {
      return
    }

    if !isStraight && uniqueCards[uniqueBestCard-1].NumValue == CardAce &&
      gotStraight(&uniqueCards, player, 4, true) {
      assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
    }
  }

  if haveFlush {
    gotFlush(cards, player, true)

    assert(player.Hand.Rank == RankFlush, "player should have a flush")

    return
  }

  if player.Hand.Rank == RankStraight {
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

    player.Hand.Rank = RankTrips
    player.Hand.Cards = kickers

    return
  }

  // two pair & pair search
  if matchHands.pairs != nil {
    if len(matchHands.pairs) > 1 {
      player.Hand.Rank = RankTwoPair
      highPairIdx := int(matchHands.pairs[len(matchHands.pairs)-1])
      lowPairIdx := int(matchHands.pairs[len(matchHands.pairs)-2])

      player.Hand.Cards = append(player.Hand.Cards, cards[lowPairIdx:lowPairIdx+2]...)
      player.Hand.Cards = append(player.Hand.Cards, cards[highPairIdx:highPairIdx+2]...)

      kicker := top_cards(cards, 1, []int{cards[highPairIdx].NumValue,
        cards[lowPairIdx].NumValue})
      player.Hand.Cards = append(kicker, player.Hand.Cards...)
    } else {
      player.Hand.Rank = RankPair
      pairidx := matchHands.pairs[0]
      kickers := top_cards(cards, 3, []int{cards[pairidx].NumValue})
      player.Hand.Cards = append(kickers, cards[pairidx:pairidx+2]...)
    }

    return
  }

  // muck
  player.Hand.Rank = RankHighCard
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
    CardTwo:   "2",
    CardThree: "3",
    CardFour:  "4",
    CardFive:  "5",
    CardSix:   "6",
    CardSeven: "7",
    CardEight: "8",
    CardNine:  "9",
    CardTen:   "10",
    CardJack:  "J",
    CardQueen: "Q",
    CardKing:  "K",
    CardAce:   "A",
  }

  name := cardNumStringMap[card.NumValue]
  if name == "" {
    fmt.Println("cardNumToString(): BUG")
    fmt.Printf("c: %s %d %d\n", card.Name, card.NumValue, card.Suit)
    return errors.New("cardNumToString")
  }

  cardSuitStringMap := map[int][]string{
    SuitClub:    {"♣", "clubs"},
    SuitDiamond: {"♦", "diamonds"},
    SuitHeart:   {"♥", "hearts"},
    SuitSpade:   {"♠", "spades"},
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
