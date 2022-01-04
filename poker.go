package main

import (
  "fmt"
  "flag"
  "net"
  "bufio"
  "bytes"
  "encoding/gob"
  "io"
  "os"
  "strconv"
  "sort"
  "errors"
  "math/rand"
  "time"
)

// ranks
const (
  R_MUCK     = iota - 1
  R_HIGHCARD
  R_PAIR
  R_2PAIR
  R_THREES
  R_STRAIGHT
  R_FLUSH
  R_FULLHOUSE
  R_FOURS
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
  S_CLUB    = iota
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
  IsSuited bool
  Suit int
  IsPair bool
  CombinedNumValue int
  Cards Cards
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
  Rank      int
  Kicker    int
  Cards     Cards
}

func (hand *Hand) RankName() string {
  switch hand.Rank {
  case R_MUCK:
    return "muck"
  case R_HIGHCARD:
    return "high card"
  case R_PAIR:
    return "pair"
  case R_2PAIR:
    return "two pair"
  case R_THREES:
    return "three of a kind"
  case R_STRAIGHT:
    return "straight"
  case R_FLUSH:
    return "flush"
  case R_FULLHOUSE:
    return "full house"
  case R_FOURS:
    return "four of a kind"
  case R_STRAIGHTFLUSH:
    return "straight flush"
  case R_ROYALFLUSH:
    return "royal flush"
  default:
    panic("RankName(): BUG")
  }
}

type Action struct {
  Action int
  Amount uint
}

type Player struct {
  Name      string
  IsCPU     bool

  IsVacant  bool

  ChipCount uint
  Hole     *Hole
  Hand     *Hand
  Action    Action
}

func (player *Player) Init(name string, isCPU bool) error {
  player.Name  = name
  player.IsCPU = isCPU

  player.IsVacant = true

  player.Action = Action{ Action: NETDATA_VACANTSEAT }

  player.ChipCount = 1e5 // XXX
  player.NewCards()

  return nil
}

func (player *Player) NewCards() {
  player.Hole  = &Hole{ Cards: make(Cards, 0, 2) }
  player.Hand  = &Hand{ Rank: R_MUCK, Cards: make(Cards, 0, 5) }
}

func (player *Player) ActionToString() string {
  switch player.Action.Action {
  case NETDATA_ALLIN:
    return "all in"
  case NETDATA_BET:
    return "raise"
  case NETDATA_CALL:
    return "call"
  case NETDATA_CHECK:
    return "check"
  case NETDATA_FOLD:
    return "fold"

  case NETDATA_VACANTSEAT:
    return "seat is open" // XXX 
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
      curcard := &Card{ Suit: suit, NumValue: c_num }
      if err := card_num2str(curcard); err != nil {
          return err
      }

      deck.cards[deck.pos] = curcard
      deck.pos++
    }
  }

  deck.pos = 0

  return nil
}

func (deck *Deck) Shuffle() {
  // XXX: get better rands
  rand.Seed(time.Now().UnixNano())
  for i := 0; i < 52; i++ {
    randidx := rand.Intn(52)
    // swap
    deck.cards[randidx], deck.cards[i] = deck.cards[i], deck.cards[randidx]
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
)

type Table struct {
  deck        *Deck       // deck of cards
  Community    Cards      // community cards
  _comsorted   Cards      // sorted community cards

  Pot          uint       // table pot
  Ante         uint       // current ante TODO allow both ante & blind modes
  Bet          uint       // current bet

  Dealer      *Player     // current dealer
  SmallBlind  *Player     // current small blind
  BigBlind    *Player     // current big blind

  players      []*Player   // array of players at table
  Winners      []*Player  // array of round winners
  curPlayer   *Player     // keeps track of whose turn it is
  better      *Player     // last player to (re-)raise
  NumPlayers   uint       // number of current players
  NumSeats     uint       // number of total possible players

  WinInfo      string // XXX tmp

  State        TableState // current status of table
  CommState    TableState // current status of community
  NumConnected uint       // number of people (players+spectators) currently at table (online mode)
}

func (table *Table) Init(deck *Deck, CPUPlayers []bool) error {
  table.deck = deck
  table.Ante = 10

  table.newCommunity()

  table.players = make([]*Player, table.NumSeats, 7) // 2 players min, 7 max

  for i := uint(0); i < table.NumSeats; i++ {
    player := &Player{}
    if err := player.Init(fmt.Sprintf("p%d", i), CPUPlayers[i]); err != nil {
      return err
    }

    table.players[i] = player
  }

  table.Dealer     = table.players[0]
  table.SmallBlind = table.players[1]
  table.BigBlind   = table.players[2]

  return nil
}

func (table *Table) newCommunity() {
  table.Community  = make(Cards, 0, 5)
  table._comsorted = make(Cards, 0, 5)
}

func (table *Table) TableStateToString() string {
  switch table.State {
  case TABLESTATE_NOTSTARTED:
    return "waiting for start"
  case TABLESTATE_PREFLOP:
    return "pre flop"
  case TABLESTATE_FLOP:
    return "flop"
  case TABLESTATE_TURN:
    return "turn"
  case TABLESTATE_RIVER:
    return "river"
  case TABLESTATE_ROUNDS:
    return "betting rounds"
  case TABLESTATE_PLAYERRAISED:
    return "player raised"
  case TABLESTATE_DONEBETTING:
    return "finished betting"
  case TABLESTATE_SHOWHANDS:
    return "showing hands"
  case TABLESTATE_ROUNDOVER:
    return "round over"
  case TABLESTATE_GAMEOVER:
    return "game over"
  default:
    return "BUG: bad table state"
  }
}

func (table *Table) commState2NetDataResponse() int {
  switch table.CommState {
  case TABLESTATE_FLOP:
    return NETDATA_FLOP
  case TABLESTATE_TURN:
    return NETDATA_TURN
  case TABLESTATE_RIVER:
    return NETDATA_RIVER
  default:
    fmt.Printf("commState2NetDataResponse(): bad state `%v`\n", table.CommState)
    return NETDATA_BADREQUEST
  }
}

func (table *Table) PublicPlayerInfo(player Player) *Player {
  if (table.State != TABLESTATE_SHOWHANDS) {
    player.Hole, player.Hand = nil, nil
  }

  return &player
}

func (table *Table) getOpenSeat() *Player {
  for _, seat := range table.players {
    if seat.IsVacant {
      seat.IsVacant = false

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

func (table *Table) getNumOpenSeats() int {
  num := 0

  for _, seat := range table.players {
    if seat.IsVacant {
      num++
    }
  }

  return num
}

func (table *Table) addNewPlayers() {
  for _, player := range table.players {
    if !player.IsVacant &&
        player.Action.Action == NETDATA_MIDROUNDADDITION {
      player.Action.Action = NETDATA_FIRSTACTION
    }
  }
}

// reorders the players slice to
//  [B+1, p..., D, S, B] pre-flop
//  [S, B, p..., D] post-flop
func (table *Table) reorderPlayers() {
  defer func() {
    fmt.Printf("reorderPlayers(): setting curPlayer to %s\n", table.players[0].Name)
    table.curPlayer = table.players[0]
  }()

  players := table.getOccupiedSeats() // XXX race cond possible

  var lastPlayer *Player

  if table.State == TABLESTATE_NEWROUND || 
     table.State == TABLESTATE_PREFLOP {
    if players[len(players)-1] == table.BigBlind {
      return
    }

    lastPlayer = table.BigBlind
  } else { // post-flop
    if players[len(players)-1] == table.Dealer {
      return
    }

    lastPlayer = table.Dealer
  }

  lastPlayerIdx := 0
  for i, player := range players {
    if player == lastPlayer { 
      lastPlayerIdx = i
      break
    }
  }

  reOrderedPlayers := append(players[lastPlayerIdx+1:],
                            players[:lastPlayerIdx]...)
  reOrderedPlayers  = append(reOrderedPlayers, players[lastPlayerIdx])
  
  fmt.Printf("reorderPlayers(): D=%s S=%s B=%s [ ", table.Dealer.Name, table.SmallBlind.Name, table.BigBlind.Name)
  for _, p := range table.players {
    fmt.Printf("%s ", p.Name)
  } ; fmt.Printf("] => [ ")
  
  table.players = append(reOrderedPlayers, table.getUnoccupiedSeats()...) // TODO use an activePlayers field instead

  for _, p := range table.players {
    fmt.Printf("%s ", p.Name)
  } ; fmt.Println("]")
}

// TODO: use a linked list?
//
// rotates the dealer and blinds
func (table *Table) rotatePlayers() {
  players := table.getOccupiedSeats()

  if table.State == TABLESTATE_NOTSTARTED || len(players) < 2 {
    return
  }

  fmt.Printf("rotateBlinds(): D=%s S=%s B=%s => ",
    table.Dealer.Name,
    table.SmallBlind.Name,
    table.BigBlind.Name)

  defer func() {
    fmt.Printf("D=%s S=%s B=%s\n",
      table.Dealer.Name,
      table.SmallBlind.Name,
      table.BigBlind.Name)

    table.reorderPlayers()
  }()

  if len(players) == 2 {
    if players[0] == table.Dealer {
      table.Dealer     = players[1]
      table.SmallBlind = table.Dealer
      table.BigBlind   = players[0]
    } else {
      table.Dealer     = players[0]
      table.SmallBlind = table.Dealer
      table.BigBlind   = players[1]
    }

    return
  }

  for i, player := range players {
    if player == table.Dealer {
      if i == len(players)-1 {
      // [ S, B, u..., D] 
        table.Dealer     = players[0]
        table.SmallBlind = players[1]
        table.BigBlind   = players[2]
      } else {
        table.Dealer = players[i+1]

        if i+1 == len(players)-1 {
        // [ B, u..., D, S ]
          table.SmallBlind = players[0]
          table.BigBlind   = players[1]
        } else if i+2 == len(players)-1 {
        // [ u..., D, S, B ]
          table.SmallBlind = players[i+2]
          table.BigBlind   = players[0]
        } else {
          table.SmallBlind = players[i+2] 
          table.BigBlind   = players[i+3]
        }
      }

      return
    }
  }

  panic("BUG: rotatePlayers(): table dealer not found")
}

// TODO: use a linked list?
func (table *Table) setNextPlayerTurn() {
  fmt.Printf("setNextPl called, curP: %s\n", table.curPlayer.Name)
  if table.State == TABLESTATE_NOTSTARTED {
    return
  }

  players := table.getNonFoldedPlayers()

  if len(players) == 1 {
    table.State = TABLESTATE_ROUNDOVER // XXX

    return
  }

  if table.curPlayer.Action.Action == NETDATA_FOLD {
    otherPlayers := table.getActiveSeats()

    for i, player := range otherPlayers {
      if player == table.curPlayer { 
        // check successive players for a non-folder
        for _, op := range otherPlayers[i+1:] {
          if op.Action.Action != NETDATA_FOLD {
            fmt.Printf("%s folded setting curP to %s\n", player.Name, op.Name)
            table.curPlayer = op
            break
          } else {
            fmt.Printf("%s FOLDED\n", player.Name)
          }
        }

        if table.State != TABLESTATE_PLAYERRAISED &&
           table.curPlayer == player {
          // folder is last active player and didn't raise, betting is done
          table.State = TABLESTATE_DONEBETTING
          table.curPlayer = table.getNonFoldedPlayers()[0] // XXX 
          return
        }

        if table.State  == TABLESTATE_PLAYERRAISED &&
           table.better == table.curPlayer           {
        // last player did not re-raise, betting is done
          table.State  = TABLESTATE_DONEBETTING
          table.better = nil // XXX 
          table.curPlayer = table.getNonFoldedPlayers()[0] // XXX 
          return
        } 

        if table.curPlayer == player {
          // if curPlayers wasn't incremented, it means that all the successive
          // players had already folded and the next active player is located [:i]
          for _, player := range otherPlayers {
            if player.Action.Action != NETDATA_FOLD {
              table.curPlayer = player
              break
            }
          }

          assert(table.curPlayer != player, "BUG: curPlayer not incremented after a fold")

          if (table.curPlayer == table.better) {
            table.State = TABLESTATE_DONEBETTING
            table.better = nil
            return
          }
        }

        return
      }
    }

    return // XXX
  }

  for i, player := range players {
    if player == table.curPlayer {
      if i == len(players)-1 {
        // [p..., curP]
        fmt.Printf("%s is last player\n", player.Name)
        fmt.Printf("[ ")
        for _, p := range players {
          fmt.Printf("%s ", p.Name)
        } ;fmt.Println("]")
        table.curPlayer = players[0]

        if table.better == nil {
          table.State = TABLESTATE_DONEBETTING
          table.curPlayer = table.getNonFoldedPlayers()[0] // XXX 
          return
        } else {
          fmt.Printf("better: %s\n", table.better.Name)
        }
      } else {
        table.curPlayer = players[i+1]
      }

      if table.State  == TABLESTATE_PLAYERRAISED &&
         table.better == table.curPlayer         &&
         player.Action.Action != NETDATA_BET {
        // last player did not re-raise, round over
          table.State  = TABLESTATE_DONEBETTING
          table.better = nil // XXX 
          table.curPlayer = table.getNonFoldedPlayers()[0] // XXX 
          return
      } 
      return
    }
  }

  panic("setNextPlayerTurn() player not found?")
}

func (table *Table) PlayerAction(player *Player, action Action) error {
  if table.State == TABLESTATE_NOTSTARTED {
    return errors.New("game has not started yet")
  }

  if table.State != TABLESTATE_ROUNDS       &&
     table.State != TABLESTATE_PLAYERRAISED &&
     table.State != TABLESTATE_PREFLOP {
       // XXX
       return errors.New("invalid table state: " + table.TableStateToString())
  }

  var amt uint = 0

  if table.State == TABLESTATE_PREFLOP {
    if player == table.BigBlind {
      amt += table.Ante
    }

    if player == table.SmallBlind {
      amt += table.Ante / 2
    }
  }

  switch action.Action {
  case NETDATA_ALLIN:
    table.Pot        += player.ChipCount
    player.ChipCount  = 0

    if action.Amount > table.Bet {
      table.Bet = action.Amount
    }
  case NETDATA_BET:
    if amt + action.Amount > player.ChipCount {
      return errors.New("not enough chips")
    }

    if amt + action.Amount == player.ChipCount {
      player.Action.Action = NETDATA_ALLIN
    } else {
      player.Action.Action = NETDATA_BET
    }

    amt              += action.Amount
    player.ChipCount -= amt
    table.Pot        += amt
    table.Bet        += amt

    table.better = player
    table.State = TABLESTATE_PLAYERRAISED
  case NETDATA_CALL:
    if table.Bet >= player.ChipCount {
      player.Action.Action  = NETDATA_ALLIN

      table.Pot            += player.ChipCount
      player.ChipCount      = 0
    } else {
      if table.State != TABLESTATE_PLAYERRAISED {
        return errors.New("nothing to call")
      }

      player.Action.Action = NETDATA_CALL

      table.Pot        += table.Bet 
      player.ChipCount -= table.Bet
    }
  case NETDATA_CHECK:
    if table.State == TABLESTATE_PLAYERRAISED {
      return errors.New("must call")
    }

    player.Action.Action = NETDATA_CHECK
  case NETDATA_FOLD:
    player.Action.Action = NETDATA_FOLD

    table.Pot        += amt
    player.ChipCount -= amt
  default:
    return errors.New("BUG: invalid player action: " + strconv.Itoa(action.Action))
  }

  table.setNextPlayerTurn()

  return nil
}

func (table *Table) Deal() {
  for _, player := range table.getActiveSeats() {
    hole       := player.Hole
    hole.Cards  = append(hole.Cards, table.deck.Pop())
    hole.Cards  = append(hole.Cards, table.deck.Pop())

    hole.FillHoleInfo()
  }

  table.State = TABLESTATE_PREFLOP
}

func (table *Table) AddToCommunity(card *Card) {
  table.Community  = append(table.Community,  card)
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
  cards_sort(&table._comsorted)
}

func (table *Table) nextCommunityAction() {
  switch table.CommState {
  case TABLESTATE_PREFLOP:
    table.DoFlop()

    table.CommState = TABLESTATE_FLOP
    table.reorderPlayers()
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
    table.Deal()

    table.CommState = TABLESTATE_PREFLOP
  case TABLESTATE_NEWROUND:
    table.rotatePlayers()
    table.Deal()

    table.CommState = TABLESTATE_PREFLOP
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
func check_ties(players []*Player, cardidx int) []*Player {
  if len(players) == 1 || cardidx == -1 {
  // one player left or remaining players tied fully
    return players
  }

  best := []*Player{ players[0] }

  for _, player := range players[1:] {
    if player.Hand.Cards[cardidx].NumValue == best[0].Hand.Cards[cardidx].NumValue {
      best = append(best, player)
    } else if player.Hand.Cards[cardidx].NumValue > best[0].Hand.Cards[cardidx].NumValue {
        best = []*Player{ player }
    }
  }

  return check_ties(best, cardidx-1)
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

  for _, player := range table.getActiveSeats() {
    player.NewCards()

    player.Action.Action = NETDATA_FIRSTACTION
  }

  table.newCommunity()

  table.better  = nil
  table.Ante   *= 2
  table.Pot     = 0 // XXX
  table.State   = TABLESTATE_NEWROUND
}

func (table *Table) finishRound() {
  players := table.getNonFoldedPlayers()

  if (len(players) == 1) {
    table.State = TABLESTATE_ROUNDOVER

    table.Winners = players
    return
  }

  table.State = TABLESTATE_SHOWHANDS

  bestPlayers := table.BestHand(players)

  if len(bestPlayers) == 1 {
    bestPlayers[0].ChipCount += table.Pot
  } else {
    splitChips := table.Pot / uint(len(bestPlayers))

    for _, player := range bestPlayers {
      player.ChipCount += splitChips
    }

    table.State = TABLESTATE_SPLITPOT
  }

  table.Winners = players
}

func (table *Table) BestHand(players []*Player) []*Player {
  table.WinInfo = ""
  for _, player := range players {
    assemble_best_hand(table, player)

    table.WinInfo += fmt.Sprintf("%s [%4s][%4s] => %-15s (rank %d)\n",
      player.Name,
      player.Hole.Cards[0].Name, player.Hole.Cards[1].Name,
      player.Hand.RankName(), player.Hand.Rank)

    fmt.Printf("%s [%4s][%4s] => %-15s (rank %d)\n", player.Name,
      player.Hole.Cards[0].Name, player.Hole.Cards[1].Name,
      player.Hand.RankName(), player.Hand.Rank)
  }

  best_players := []*Player{ players[0] }
  for _, player := range players[1:] {
    if player.Hand.Rank == best_players[0].Hand.Rank {
      best_players = append(best_players, player)
    } else if player.Hand.Rank > best_players[0].Hand.Rank {
        best_players = []*Player{ player }
    }
  }

  tied_players := check_ties(best_players, 4)

  if len(tied_players) > 1 {
    // split pot
    fmt.Printf("split pot between ")
    table.WinInfo += "split pot between "
    for _, player := range tied_players {
      fmt.Printf("%s ", player.Name)
      table.WinInfo += player.Name + " "
    } ; fmt.Printf("\r\n")

    table.WinInfo += "\nwinning hand => " + tied_players[0].Hand.RankName() + "\n" 
    fmt.Printf("winning hand => %s\n", tied_players[0].Hand.RankName())
  } else {
    table.WinInfo += "\n" + tied_players[0].Name + "  wins with " + tied_players[0].Hand.RankName() + "\n"
    fmt.Printf("\n%s wins with %s\n", tied_players[0].Name, tied_players[0].Hand.RankName())
  }

  // print the best hand
  for _, card := range tied_players[0].Hand.Cards {
      fmt.Printf("[%4s]", card.Name)
      table.WinInfo += fmt.Sprintf("[%4s]", card.Name)
  } ; fmt.Println()

  return tied_players
}

// hand matching logic unoptimized
func assemble_best_hand(table *Table, player *Player) {
  cards := append(table.Community, player.Hole.Cards...)
  cards_sort(&cards)
  bestcard := len(cards)

  // get all the pairs/threes/fours into one slice
  // NOTE: ascending order
  //
  // this struct keeps a slice of the match type indexes
  // in ascending order
  var match_hands struct {
    fours  []uint
    threes []uint
    pairs  []uint
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
        matchmemb = &match_hands.fours
      case 3:
        matchmemb = &match_hands.threes
      case 2:
        matchmemb = &match_hands.pairs
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

    for i := len(cards)-1; i >= 0; i-- {
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
  got_flush := func(cards Cards, player *Player, add_to_cards bool) (bool, int) {
    type _suitstruct struct{cnt uint; cards Cards}
    suits := make(map[int]*_suitstruct)

    for _, card := range cards {
      suits[card.Suit] = &_suitstruct{ cards: Cards{} }
    }
    // count each suit
    for _, card := range cards {
      suits[card.Suit].cnt++
      suits[card.Suit].cards = append(suits[card.Suit].cards, card)
    }
    // search for flush
    for suit, suit_struct := range suits {
      if suit_struct.cnt >= 5 { // NOTE: it's only possible to get one flush
        player.Hand.Rank  = R_FLUSH
        if add_to_cards {
          player.Hand.Cards = append(player.Hand.Cards, suit_struct.cards[len(suit_struct.cards)-5:len(suit_struct.cards)]...)
        }
        return true, suit
      }
    }

    return false, 0
  }

  // straight flush/straight search function //
  got_straight := func(cards *Cards, player *Player, high int, acelow bool) (bool) {
    straight_flush := true
    if acelow {
    // check ace to 5
      acesuit := (*cards)[len(*cards)-1].Suit
      for i := 1; i <= high; i++ {
        if (*cards)[i].Suit != acesuit {
          straight_flush = false
        }
        if (*cards)[i].NumValue != (*cards)[i-1].NumValue+1 {
          return false
        }
      }
    } else {
      low := high-4
      for i := high; i > low; i-- {
        //fmt.Printf("h %d L %d ci %d ci-1 %d\n", high, low, i, i-1)
        if (*cards)[i].Suit != (*cards)[i-1].Suit+1 {
          straight_flush = false
        }
        if (*cards)[i].NumValue != (*cards)[i-1].NumValue+1 {
          return false
        }
      }
    }
    if straight_flush {
      if (*cards)[high].NumValue == C_ACE {
        player.Hand.Rank = R_ROYALFLUSH
      } else {
        player.Hand.Rank = R_STRAIGHTFLUSH
      }
    } else {
      player.Hand.Rank = R_STRAIGHT
    }
    player.Hand.Cards = append(player.Hand.Cards, (*cards)[high-4:high+1]...)
    assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

    return true
  }

  if !matching_cards {
   // best possible hands with no matches in order:
   // royal flush, straight flush, flush, straight or high card.
    // XXX: make better
    // we check for best straight first to reduce cycles
    for i := 1; i < 4; i++ {
      if got_straight(&cards, player, bestcard-i, false) {
        return
      }
    }
    if cards[len(cards)-1].NumValue == C_ACE &&
       got_straight(&cards, player, 4, true) {
      return
    }
    if player.Hand.Rank == R_STRAIGHTFLUSH {
      return
    }
    have_flush, _ := got_flush(cards, player, true)
    if have_flush || player.Hand.Rank == R_STRAIGHT {
      return
    }
    // muck
    player.Hand.Rank   = R_HIGHCARD
    player.Hand.Cards  = append(player.Hand.Cards, cards[bestcard-1],
                                cards[bestcard-2], cards[bestcard-3],
                                cards[bestcard-4], cards[bestcard-5])
    assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
    return
  }

  // fours search //
  if match_hands.fours != nil {
    foursidx := int(match_hands.fours[0]) // 0 because it's impossible to
                                          // get fours twice
    kicker := &Card{}
    for i := bestcard-1; i >= 0; i-- { // kicker search
      if cards[i].NumValue > cards[foursidx].NumValue {
        kicker = cards[i]
        break
      }
    }
   assert(kicker != nil, "fours: kicker == nil")
   player.Hand.Rank  = R_FOURS
   player.Hand.Cards = append(Cards{kicker}, cards[foursidx:foursidx+4]...)
   return
  }

  // fullhouse search //
  //
  // NOTE: we check for a fullhouse before a straight flush because it's
  // impossible to have both at the same time and searching for the fullhouse
  // first saves some cycles+space
  if match_hands.threes != nil && match_hands.pairs != nil {
    player.Hand.Rank = R_FULLHOUSE
    pairidx   := int(match_hands.pairs [len(match_hands.pairs )-1])
    threesidx := int(match_hands.threes[len(match_hands.threes)-1])
    player.Hand.Cards = append(player.Hand.Cards, cards[pairidx:pairidx+2]...)
    player.Hand.Cards = append(player.Hand.Cards, cards[threesidx:threesidx+3]...)
    assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
    return
  }

  // flush search //
  //
  // NOTE: we search for the flush here to ease the straight flush logic
  have_flush, suit := got_flush(cards, player, false)

  // remove duplicate card (by number) for easy straight search
  unique_cards  := Cards{}

  if have_flush {
  // check for possible RF/straight flush suit
    cardmap := make(map[int]int) // key == num, val == suit
    for _, card := range cards {
      mappedsuit, found := cardmap[card.NumValue];
      if found && mappedsuit != suit && card.Suit == suit {
        cardmap[card.NumValue] = card.Suit
        assert(unique_cards[len(unique_cards)-1].NumValue == card.NumValue, "unique_cards problem")
        unique_cards[len(unique_cards)-1] = card // should _always_ be last card
      } else if !found {
        cardmap[card.NumValue] = card.Suit
        unique_cards = append(unique_cards, card)
      }
    }
    assert((len(unique_cards) <= 7 && len(unique_cards) >= 3),
           fmt.Sprintf("impossible number of unique cards (%v)", len(unique_cards)))
  } else {
    cardmap := make(map[int]bool)
    for _, card := range cards {
      if _, val := cardmap[card.NumValue]; !val {
        cardmap[card.NumValue] = true
        unique_cards = append(unique_cards, card)
      }
    }
    assert((len(unique_cards) <= 7 && len(unique_cards) >= 1),
           "impossible number of unique cards")
  }

  // RF, SF and straight search //
  if len(unique_cards) >= 5 {
    unique_bestcard := len(unique_cards)
    iter := unique_bestcard - 4
    //fmt.Printf("iter %v len(uc) %d\n)", iter, len(unique_cards))
    for i := 1; i <= iter; i++ {
      if got_straight(&unique_cards, player, unique_bestcard-i, false) {
        assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
        return
      }
    }
    if unique_cards[unique_bestcard-1].NumValue == C_ACE &&
       got_straight(&unique_cards, player, 4, true) {
      assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
      return
    }
  }

  // threes search
  if match_hands.threes != nil {
    firstcard := int(match_hands.threes[len(match_hands.threes)-1])

    threeslice := make(Cards, 0, 3)
    threeslice  = append(threeslice, cards[firstcard:firstcard+3]...)

    kickers := top_cards(cards, 2, []int{cards[firstcard].NumValue})
    // order => [kickers][threes]
    kickers = append(kickers, threeslice...)

    player.Hand.Rank  = R_THREES
    player.Hand.Cards = kickers
    return
  }

  // two pair & pair search
  if match_hands.pairs != nil {
    if len(match_hands.pairs) > 1 {
      player.Hand.Rank   = R_2PAIR
      highpairidx := int(match_hands.pairs[len(match_hands.pairs)-1])
      lowpairidx  := int(match_hands.pairs[len(match_hands.pairs)-2])

      player.Hand.Cards = append(player.Hand.Cards, cards[lowpairidx:lowpairidx+2]...)
      player.Hand.Cards = append(player.Hand.Cards, cards[highpairidx:highpairidx+2]...)

      kicker := top_cards(cards, 1, []int{cards[highpairidx].NumValue,
                                          cards[lowpairidx ].NumValue})
      player.Hand.Cards = append(kicker, player.Hand.Cards...)
    } else {
      player.Hand.Rank = R_PAIR
      pairidx := match_hands.pairs[0]
      kickers := top_cards(cards, 3, []int{cards[pairidx].NumValue})
      player.Hand.Cards = append(kickers, cards[pairidx:pairidx+2]...)
    }
    return
  }

  // muck
  player.Hand.Rank   = R_HIGHCARD
  player.Hand.Cards = append(player.Hand.Cards, cards[bestcard-1],
                             cards[bestcard-2], cards[bestcard-3],
                             cards[bestcard-4], cards[bestcard-5])

  return
}

func cards_sort(cards *Cards) error {
  sort.Slice((*cards), func(i, j int) bool {
    return (*cards)[i].NumValue < (*cards)[j].NumValue
  })

  return nil
}

func card_num2str(card *Card) error {
  var name, suit, suit_full string
  switch card.NumValue {
  case C_TWO:
    name  = "2"
  case C_THREE:
    name = "3"
  case C_FOUR:
    name = "4"
  case C_FIVE:
    name = "5"
  case C_SIX:
    name = "6"
  case C_SEVEN:
    name = "7"
  case C_EIGHT:
    name = "8"
  case C_NINE:
    name = "9"
  case C_TEN:
    name = "10"
  case C_JACK:
    name = "J"
  case C_QUEEN:
    name = "Q"
  case C_KING:
    name = "K"
  case C_ACE:
    name = "A"
  default:
    fmt.Println("card_num2str(): BUG")
    fmt.Printf("c: %s %d %d\n", card.Name, card.NumValue, card.Suit)
    return errors.New("card_num2str")
  }

  switch card.Suit {
  case S_CLUB:
    suit  = "♣"
    suit_full = "clubs"
  case S_DIAMOND:
    suit  = "♦"
    suit_full = "diamonds"
  case S_HEART:
    suit  = "♥"
    suit_full = "hearts"
  case S_SPADE:
    suit  = "♠"
    suit_full = "spades"
  }

  card.Name     = name + " "    + suit
  card.FullName = name + " of " + suit_full

  return nil
}

func assert(cond bool, msg string) {
  if !cond {
    panic(msg)
  }
}

const (
  NETDATA_CLOSE = iota
  NETDATA_NEWCONN

  NETDATA_NEWPLAYER
  NETDATA_CURPLAYERS
  NETDATA_PLAYERLEFT
  NETDATA_CLIENTEXITED

  NETDATA_MAKEADMIN
  NETDATA_STARTGAME

  NETDATA_PLAYERACTION
  NETDATA_ALLIN
  NETDATA_BET
  NETDATA_CALL
  NETDATA_CHECK
  NETDATA_RAISE
  NETDATA_FOLD
  NETDATA_FLOP

  NETDATA_SHOWHAND

  NETDATA_FIRSTACTION
  NETDATA_MIDROUNDADDITION
  NETDATA_VACANTSEAT

  NETDATA_DEAL
  NETDATA_TURN
  NETDATA_RIVER
  NETDATA_BESTHAND
  NETDATA_ROUNDOVER

  NETDATA_BADREQUEST
)

type NetData struct {
  Request     int
  Response    int
  Msg         string // server msg
  Table      *Table
  PlayerData *Player
}

func (netData *NetData) Init() {
  return
}

func sendData(data *NetData, writeConn *bufio.Writer) {
  if data == nil {
    panic("sendData(): data == nil")
  }

  if writeConn == nil {
    panic("sendData(): writeConn == nil")
  }

  var gobBuf bytes.Buffer
  enc := gob.NewEncoder(&gobBuf)

  enc.Encode(data)

  writeConn.Write(gobBuf.Bytes())
  writeConn.Flush()
}

func serverCloseConn(conn net.Conn) {
  fmt.Printf("<= closing conn to %s\n", conn.RemoteAddr().String())
  conn.Close()
}

func runServer(table *Table, port string) (err error) {
  listen, err := net.Listen("tcp", port)
  if err != nil {
    return err
  }
  defer listen.Close()

  connectedClientMap := make(map[*net.Conn]*bufio.Writer)
  playerMap := make(map[*net.Conn]*Player)
  var tableAdmin *net.Conn

  sendResponseToAll := func(data *NetData, except *bufio.Writer) {
    for _, clientWriter := range connectedClientMap {
      if clientWriter != except {
        sendData(data, clientWriter)
      }
    }
  }

  removeClient := func(conn *net.Conn) {
    netData := &NetData{
        Response: NETDATA_CLIENTEXITED,
        Table:    table,
    }

    delete(connectedClientMap, conn)

    table.NumConnected--
    sendResponseToAll(netData, nil)
  }

  removePlayer := func(conn *net.Conn) {
    player := playerMap[conn]

    delete(playerMap, conn)

    if player != nil { // else client was a spectator
      fmt.Printf("removing %s\n", player.Name)

      if tableAdmin == conn {
        tableAdmin = nil
      }
      
      player.IsVacant      = true
      player.Action.Action = NETDATA_VACANTSEAT

      netData := &NetData{
        Response:   NETDATA_PLAYERLEFT,
        Table:      table,
        PlayerData: player,
      }

      sendResponseToAll(netData, connectedClientMap[conn])
    }
  }

  sendPlayerActionToAll := func(player *Player) {
    fmt.Printf("%s action => %s\n", player.Name, player.ActionToString())

    netData := &NetData{ 
     Response:   NETDATA_PLAYERACTION,
     Table:      table,
     PlayerData: player,
    }

    sendResponseToAll(netData, nil)
  }

  sendDeals := func() {
    netData := &NetData{ Response: NETDATA_DEAL }

    for conn, player := range playerMap {
      netData.PlayerData = player

      sendData(netData, connectedClientMap[conn])
    }
  }

  sendHands := func() {
    netData := &NetData{ Response: NETDATA_SHOWHAND }

    for _, player := range table.getNonFoldedPlayers() {
      netData.PlayerData = table.PublicPlayerInfo(*player)

      var conn *net.Conn
      for k, v := range playerMap {
        if v == player {
          conn = k
          break
        }
      }
      assert(conn != nil, "sendPlayerActionToAll(): player not in playerMap")

      sendResponseToAll(netData, connectedClientMap[conn])
    }
  }

  fmt.Printf("starting server on port %v\n", port)

  for {
    conn, err := listen.Accept()
    if err != nil {
      return err
    }

    table.NumConnected++ // XXX: racy?

    go func(conn net.Conn) {
      defer serverCloseConn(conn)
      defer removeClient(&conn)
      defer removePlayer(&conn)

      var (
        readBuf   = make([]byte, 4096)
        readConn  = bufio.NewReader(conn)
        writeConn = bufio.NewWriter(conn)
      )

      fmt.Printf("=> new conn from %s\n", conn.RemoteAddr().String())

      connectedClientMap[&conn] = writeConn

      netData := NetData{
        Response: NETDATA_NEWCONN,
        Table:    table,
      }

      for {
        n, err := readConn.Read(readBuf); if err != nil {
          if err == io.EOF {
            fmt.Println("!! EOF 1")
          }

          fmt.Printf("runServer(): readConn err: %v\n", err)

          return
	      }

        rawData := bytes.NewReader(readBuf[:n])

        switch err {
        case io.EOF:
          fmt.Println("!! EOF 2")
        case nil:
          // we need to set Table member to node otherwise gob will
          // modify our table structure if a user sends that member
          netData = NetData{ Response: NETDATA_NEWCONN, Table: nil }

          gob.NewDecoder(rawData).Decode(&netData)

          netData.Table = table 

          if netData.Request == NETDATA_NEWCONN {
            sendResponseToAll(&netData, nil)

            // send current player info to this client
            if table.NumConnected > 1 {
                netData.Response = NETDATA_CURPLAYERS
                netData.Table    = table

                for _, player := range table.getOccupiedSeats() {
                  netData.PlayerData = table.PublicPlayerInfo(*player)
                  sendData(&netData, writeConn)
                  time.Sleep(50 * time.Millisecond) // XXX why do i need to do this
                }
            }

            if player := table.getOpenSeat(); player != nil {
              fmt.Printf("adding %p as player %s\n", &conn, player.Name)
              table.NumPlayers++ // XXX racy?

              if table.State == TABLESTATE_NOTSTARTED {
                player.Action.Action = NETDATA_FIRSTACTION
              } else {
                player.Action.Action = NETDATA_MIDROUNDADDITION
              }

              playerMap[&conn] = player

              if table.curPlayer == nil {
                table.curPlayer = player
              }

              netData.Response   = NETDATA_NEWPLAYER
              netData.Table      = table
              netData.PlayerData = table.PublicPlayerInfo(*player)

              sendResponseToAll(&netData, writeConn)
            }

            if tableAdmin == nil {
              tableAdmin = &conn
              sendData(&NetData{ Response: NETDATA_MAKEADMIN }, writeConn)
            }
          } else {
            switch netData.Request {
            case NETDATA_CLIENTEXITED:
              return
            case NETDATA_STARTGAME:
              if &conn != tableAdmin {
                netData.Response = NETDATA_BADREQUEST
                netData.Msg      = "only the table admin can do that"
                netData.Table    = nil

                sendData(&netData, writeConn)
              } else if table.NumConnected < 2 {
                netData.Response = NETDATA_BADREQUEST
                netData.Msg      = "not enough players to start"
                netData.Table    = nil

                sendData(&netData, writeConn)
              } else if table.State != TABLESTATE_NOTSTARTED {
                netData.Response = NETDATA_BADREQUEST
                netData.Msg      = "this game has already started"
                netData.Table    = nil

                sendData(&netData, writeConn)
              } else { // start game
                table.nextTableAction()

                sendDeals()
		          }
            case NETDATA_BET, NETDATA_CALL, NETDATA_CHECK, NETDATA_FOLD:
              player := playerMap[&conn]

              if player == nil {
                netData.Response = NETDATA_BADREQUEST
                netData.Msg      = "you are not a player"
                netData.Table    = nil

                sendData(&netData, writeConn)
                continue
              }

              if player != table.curPlayer {
                netData.Response = NETDATA_BADREQUEST
                netData.Msg      = "it's not your turn"
                netData.Table    = nil

                sendData(&netData, writeConn)
                continue
              }
              
              if err := table.PlayerAction(player, netData.PlayerData.Action); err != nil {
                netData.Response = NETDATA_BADREQUEST
                netData.Table    = nil
                netData.Msg      = err.Error()

                sendData(&netData, writeConn)

                continue
              } else if table.State != TABLESTATE_DONEBETTING {
                if table.State == TABLESTATE_ROUNDOVER {
                // all other players folded before all comm cards were dealt
                  table.finishRound()
                  fmt.Printf("winner # %d\n", len(table.Winners))
                  fmt.Println(table.Winners[0].Name + " wins by folds")

                  netData.Response   = NETDATA_ROUNDOVER
                  netData.Table      = table
                  netData.Msg        = table.Winners[0].Name + " wins by folds"
                  netData.PlayerData = nil

                  sendResponseToAll(&netData, nil)

                  table.newRound()
                  table.nextTableAction()
                  sendDeals()
                } else {
                  sendPlayerActionToAll(table.PublicPlayerInfo(*player))
                }
              } else { 
                sendPlayerActionToAll(table.PublicPlayerInfo(*player))

                fmt.Println("** done betting...")
                table.nextCommunityAction()

                if table.State == TABLESTATE_ROUNDOVER {
                  table.finishRound()
                  sendHands()

                  netData.Response   = NETDATA_ROUNDOVER
                  netData.Table      = table
                  netData.Msg        = table.WinInfo

                  sendResponseToAll(&netData, nil)

                  table.newRound()
                  table.nextTableAction()
                  sendDeals()

                  continue
                }

                netData.Response   = table.commState2NetDataResponse()
                netData.Table      = table
                netData.PlayerData = nil

                sendResponseToAll(&netData, nil)
              }
            default:
              netData.Response = NETDATA_BADREQUEST
              netData.Msg      = "bad request"
              netData.Table, netData.PlayerData = nil, nil
              
              sendData(&netData, writeConn)
            }

            //sendData(&netData, writeConn)
          }
        default:
          fmt.Printf("srv recv err: %s\n", err)
          return
        }
      }
    }(conn)
  }

  return nil
}

type FrontEnd interface {
  InputChan()  chan *NetData
  OutputChan() chan *NetData
  Init()       error
  Run()        error
  Finish()     chan error
}

func runClient(addr string, isGUI bool) (err error) {
  conn, err := net.Dial("tcp", addr)
  if err != nil {
    return err
  }

  defer conn.Close()

  var (
        readBuf   = make([]byte, 4096)
        readConn  = bufio.NewReader(conn)
        writeConn = bufio.NewWriter(conn)
  )

   var frontEnd FrontEnd
   if isGUI {
     ;//frontEnd := runGUI()
   } else { // CLI mode
     frontEnd = &CLI{}

     if err := frontEnd.Init(); err != nil {
       return err
     }
   }

  fmt.Printf("connected to %s\n", addr)

  go func () {
    sendData(&NetData{ Request: NETDATA_NEWCONN }, writeConn)

    for {
      var n int
      if n, err = readConn.Read(readBuf); err != nil {
        frontEnd.Finish() <- err
        return
      }

      rawData := bytes.NewReader(readBuf[:n])

      netData := &NetData{}
      dec := gob.NewDecoder(rawData)
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
      case netData := <-frontEnd.OutputChan():
        sendData(netData, writeConn)
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

    table := &Table{ NumSeats: 7 } // FIXME: tmp
    if err := table.Init(deck, []bool{false, false, false, false, false, false, false}); err != nil {
      return err
    }

    deck.Shuffle()

    if err := runServer(table, "0.0.0.0:" + opts.serverMode); err != nil {
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
    ;
  }

  /*if false {
    if err := gui_run(table); err != nil {
      fmt.Printf("gui_run() err: %v\n", err)
      return nil
    }
  }*/

  return nil
}

type options struct {
  serverMode string
  connect    string
  gui        bool
}

/*
  TODO: make a player that is added mid-round is not counted as part of current round
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
  )
  flag.Usage = func() {
    fmt.Println(usage)
    flag.PrintDefaults()
  }
  flag.StringVar(&serverMode, "s", "", "host a poker table on <port>")
  flag.StringVar(&connect, "c", "", "connect to a gopoker table")
  flag.BoolVar(&gui, "g", false, "run with a GUI")
  flag.Parse()

  opts := &options{
    serverMode: serverMode,
    connect:    connect,
    gui:        gui,
  }

  if err := runGame(opts); err != nil {
    fmt.Println(err)
    return
  }
}
