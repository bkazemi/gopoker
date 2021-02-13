package main

import (
  "fmt"
  "sort"
  "errors"
  "math/rand"
  "time"
)

// ranks
var (
  R_MUCK          = -1
  R_HIGHCARD      =  0
  R_PAIR          =  1
  R_2PAIR         =  2
  R_THREES        =  3
  R_STRAIGHT      =  4
  R_FLUSH         =  5
  R_FULLHOUSE     =  6
  R_FOURS         =  7
  R_STRAIGHTFLUSH =  8
  R_ROYALFLUSH    =  9
)

// cards
var (
  C_ACELOW = 1
  C_TWO    = 2
  C_THREE  = 3
  C_FOUR   = 4
  C_FIVE   = 5
  C_SIX    = 6
  C_SEVEN  = 7
  C_EIGHT  = 8
  C_NINE   = 9
  C_TEN    = 10
  C_JACK   = 11
  C_QUEEN  = 12
  C_KING   = 13
  C_ACE    = 14
)

// suits
var (
  S_CLUB    = 0
  S_DIAMOND = 1
  S_HEART   = 2
  S_SPADE   = 3
)

type Card struct {
  name     string
  fullname string
  suit     int
  numvalue int
}

type Cards []*Card

type Hand struct {
  rank      int
  kicker    int
  cards     Cards
  numvalue  int
}

func (hand *Hand) RankName() string {
  switch hand.rank {
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

type Player struct {
  name      string
  chipcount uint
  hole      Cards
  hand     *Hand
}

func (player *Player) Init(name string) error {
  player.name       = name
  player.hole       = make(Cards, 0, 2)
  player.hand       = &Hand{ rank: R_MUCK }
  player.hand.cards = make(Cards, 0, 5)

  return nil
}

type Deck struct {
  pos        uint
  cardarray *[52]Card
}

func (deck *Deck) Init() error {
  var cards   [52]Card
  var curcard *Card

  deck.cardarray = &cards

  for suit := S_CLUB; suit <= S_SPADE; suit++ {
    for c_num := C_TWO; c_num <= C_ACE; c_num++ {
        curcard          = &deck.cardarray[deck.pos]
        curcard.numvalue = c_num
        curcard.suit     = suit
        if err := card_num2str(curcard); err != nil {
          return err
        }
        deck.pos++
    }
  }
  deck.pos       = 0
  deck.cardarray = &cards

  return nil
}

func (deck *Deck) Shuffle() {
  // XXX: get better rands
  rand.Seed(time.Now().UnixNano())
  for i := 0; i < 52; i++ {
    randidx := rand.Intn(52)
    /*tmp      = deck.cardarray[i]
    // swap current card w/ random one in deck
      deck.cardarray[i]       = deck.cardarray[randidx]
      deck.cardarray[randidx] = tmp*/
    // swap
    deck.cardarray[randidx], deck.cardarray[i] = deck.cardarray[i], deck.cardarray[randidx]
  }
  deck.pos = 0
}

// "remove" card from deck (functionally)
func (deck *Deck) Pop() *Card {
  deck.pos++
  return &deck.cardarray[deck.pos-1]
}

type Table struct {
  deck       *Deck     // deck of cards
  community   Cards    // community cards
  _comsorted  Cards    // sorted community cards
  pot         uint     // table pot
  ante        uint     // current ante
  bigblind   *Player   // current big blind
  smallblind *Player   // current small blind
  players     []Player // array of players at table
  numplayers  uint     // number of total players
}

func (table *Table) Init(deck *Deck) error {
  table.deck       = deck
  table.ante       = 10

  // allocate slices
  table.community  = make(Cards, 0, 5)
  table._comsorted = make(Cards, 0, 5)
  table.players    = make([]Player, table.numplayers, 6) // 2 players min, 6 max

  table.bigblind   = &table.players[0]
  table.smallblind = &table.players[1]

  for i := uint(0); i < table.numplayers; i++ {
    table.players[i].Init(fmt.Sprintf("p%d", i))
  }

  return nil
}

func (table *Table) Deal() {
  for i := 0; i < len(table.players); i++ {
     hole := &table.players[i].hole
    *hole  = append(*hole, table.deck.Pop())
    *hole  = append(*hole, table.deck.Pop())
  }
}

func (table *Table) AddToCommunity(card *Card) {
  table.community  = append(table.community, card)
  table._comsorted = append(table._comsorted, card)
}

// print name of current community cards to stdout
func (table *Table) PrintCommunity() {
  for _, card := range table.community {
    fmt.Printf(" [%s]", card.name)
  }
  fmt.Println()
}

func (table *Table) PrintSortedCommunity() {
  fmt.Printf("sorted: ")
  for _, card := range table._comsorted {
    fmt.Printf(" [%s]", card.name)
  }
  fmt.Println()
}

// sort community cards by number
func (table *Table) SortCommunity() {
  cards_sort(&table._comsorted)
}

func (table *Table) DoFlop() {
  for i := 0; i < 3; i++ {
    table.AddToCommunity(table.deck.Pop())
  }
  table.PrintCommunity()
  table.SortCommunity()
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
func check_ties(players []Player, cardidx int) []Player {
  if len(players) == 1 || cardidx == -1 {
  // one player left or remaining players tied fully
    return players
  }

  best := []Player{ players[0] }
  for _, player := range players[1:] {
    //fmt.Printf("p %v against %v\n", player.hand.cards[cardidx].name, players[0].hand.cards[cardidx].name)
    if player.hand.cards[cardidx].numvalue == best[0].hand.cards[cardidx].numvalue {
      //fmt.Printf("ct: idx %v: %v == %v\n", cardidx, player.hand.cards[cardidx].numvalue, players[0].hand.cards[cardidx].numvalue )
      best = append(best, player)
    } else if player.hand.cards[cardidx].numvalue > best[0].hand.cards[cardidx].numvalue {
        //fmt.Printf("ct: idx %v: %v > %v\n", cardidx, player.hand.cards[cardidx].numvalue, players[0].hand.cards[cardidx].numvalue )
        best = []Player{ player }
    }
  }

  return check_ties(best, cardidx-1)
}

func (table *Table) BestHand() {

  for _, player := range table.players {
    assemble_best_hand(table, &player)
    fmt.Printf("%s [%4s][%4s] => %-15s (rank %d)\n", player.name,
               player.hole[0].name, player.hole[1].name,
               player.hand.RankName(), player.hand.rank)
  }
  best_players := []Player{ table.players[0] }
  for _, player := range table.players[1:] {
    if player.hand.rank == best_players[0].hand.rank {
      best_players = append(best_players, player)
    } else if player.hand.rank > best_players[0].hand.rank {
        best_players = []Player{ player }
    }
  }

  tied_players := check_ties(best_players, 4)

  if len(tied_players) > 1 {
    // split pot
    fmt.Printf("split pot between ")
    for _, player := range tied_players {
      fmt.Printf("%s ", player.name)
    } ; fmt.Printf("\r\n")
    fmt.Printf("winning hand => %s\n", tied_players[0].hand.RankName())
  } else {
    fmt.Printf("\n%s wins with %s\n", tied_players[0].name, tied_players[0].hand.RankName())
  }
  // print the best hand
  for _, card := range tied_players[0].hand.cards {
      fmt.Printf("[%4s]", card.name)
    } ; fmt.Println()
}

// hand matching logic unoptimized
func assemble_best_hand(table *Table, player *Player) {
  cards := append(table.community, player.hole...)
  cards_sort(&cards)
  bestcard := len(cards)

  // royal flush search
  if cards[bestcard-1].numvalue == C_ACE {
    suit     := cards[bestcard-1].suit
    lastcard := C_ACE
    is_rf    := true
    for _, card := range cards[bestcard-6:bestcard-1] {
      if card.numvalue != lastcard-1 || card.suit != suit {
        is_rf = false
        break
      }
    }
    if is_rf {
      player.hand.rank = R_ROYALFLUSH
      return
    }
  }

  // get all the pairs/threes/fours into one slice
  // NOTE: ascending order
  var matching_cards Cards
  var cur_card *Card = cards[0]
  cur_card_idx      := 0
  cur_match_num     := 0

  // this struct keeps a slice of the match type indexes
  // in ascending order
  var match_hands struct {
    fours  []uint
    threes []uint
    pairs  []uint
  }

  // XXX messy.
  for i, card := range cards[1:] {
    if card.numvalue == cur_card.numvalue {
      if i == 0 || i == cur_card_idx+1 { // need this or first card in pairing wont get in
        matching_cards = append(matching_cards, cur_card)
        cur_match_num = 2
      } else {
        cur_match_num++
      }
      matching_cards = append(matching_cards, card)
      if i != len(cards[1:])-1 { // need for last elemnt
        continue
      }
    }
    var matchmemb *[]uint
    switch cur_match_num {
    case 4:
      matchmemb = &match_hands.fours
    case 3:
      matchmemb = &match_hands.threes
    case 2:
      matchmemb = &match_hands.pairs
    }
    if matchmemb != nil {
      if cur_card_idx == 0 && matching_cards[0] != cards[1] {
        *matchmemb = append(*matchmemb, uint(0))
     } else {
        *matchmemb = append(*matchmemb, uint(cur_card_idx+1))
      }
    }
    cur_card      = card
    cur_card_idx  = i
    cur_match_num = 0
  }

  // used for tie breakers
  // this func assumes the card slice is sorted and ret will be <= 5
  top_cards := func (cards Cards, num int, except []int) Cards {
    ret := make(Cards, 0, 5)
    assert(len(cards) <= 7, "too many cards in top_cards()")
    for i := len(cards)-1; i >= 0; i-- {
      skip := false
      if len(ret) == num {
        return ret
      }
      for _, except_num := range except {
        if cards[i].numvalue == except_num {
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
      suits[card.suit] = &_suitstruct{ cards: Cards{} }
    }
    // count each suit
    for _, card := range cards {
      suits[card.suit].cnt++
      suits[card.suit].cards = append(suits[card.suit].cards, card)
    }
    // search for flush
    for suit, suit_struct := range suits {
      if suit_struct.cnt >= 5 { // NOTE: it's only possible to get one flush
        player.hand.rank  = R_FLUSH
        if (add_to_cards) {
		player.hand.cards = append(player.hand.cards, suit_struct.cards[len(suit_struct.cards)-5:len(suit_struct)]...)
        }
        return true, suit
      }
    }

    return false, 0
  }

  // straight flush/straight search function //
  got_straight := func(cards *Cards, player *Player, high int, acelow bool) (bool) {
    if len(player.hand.cards) > 0 {panic("before straight is the CULPRIT!!!")}

    straight_flush := true
    if acelow {
    // check ace to 5
      acesuit := (*cards)[len(*cards)-1].suit
      for i := 1; i <= high; i++ {
        if (*cards)[i].suit != acesuit {
          straight_flush = false
        }
        if (*cards)[i].numvalue != (*cards)[i-1].numvalue+1 {
          return false
        }
      }
    } else {
      low := high-4
      for i := high; i > low; i-- {
        //fmt.Printf("h %d L %d ci %d ci-1 %d\n", high, low, i, i-1)
        if (*cards)[i].suit != (*cards)[i-1].suit+1 {
          straight_flush = false
        }
        if (*cards)[i].numvalue != (*cards)[i-1].numvalue+1 {
          return false
        }
      }
    }
    if straight_flush {
      player.hand.rank = R_STRAIGHTFLUSH
    } else {
      player.hand.rank = R_STRAIGHT
    }
    player.hand.cards = append(player.hand.cards, (*cards)[high-4:high+1]...)
    assert(len(player.hand.cards) == 5, fmt.Sprintf("%d", len(player.hand.cards)))

    return true
  }

  if len(matching_cards) == 0 {
  /* best possible hands with no matches in order:
   * straight flush, flush, straight or high card.
   */
    // XXX: make better
    // we check for best straight first to reduce cycles
    for i := 1; i < 4; i++ {
      if got_straight(&cards, player, bestcard-i, false) {
        return
      }
    }
    if cards[len(cards)-1].numvalue == C_ACE &&
       got_straight(&cards, player, 4, true) {
      return
    }
    if player.hand.rank == R_STRAIGHTFLUSH {
      return
    }
    have_flush, _ := got_flush(cards, player, true)
    if have_flush || player.hand.rank == R_STRAIGHT {
      return
    }
    // muck
    player.hand.rank   = R_HIGHCARD
    player.hand.cards  = append(player.hand.cards, cards[bestcard-1],
                                cards[bestcard-2], cards[bestcard-3],
                                cards[bestcard-4], cards[bestcard-5])
    assert(len(player.hand.cards) == 5, fmt.Sprintf("%d", len(player.hand.cards)))
    return
  }

  // fours search //
  if match_hands.fours != nil {
    foursidx := int(match_hands.fours[len(match_hands.fours)-1])
    kicker := &Card{}
    for i := bestcard-1; i >= 0; i-- { // kicker search
      if cards[i].numvalue > cards[foursidx].numvalue {
        kicker = cards[i]
        break
      }
    }
   assert(kicker != nil, "fours: kicker == nil")
   player.hand.rank  = R_FOURS
   player.hand.cards = append(Cards{kicker}, player.hand.cards...)
   return
  }

  // fullhouse search //
  //
  // NOTE: we check for a fullhouse before a straight flush because it's
  // impossible to have both at the same time and searching for the fullhouse
  // first saves some cycles+space
  if match_hands.threes != nil && match_hands.pairs != nil {
    player.hand.rank = R_FULLHOUSE
    pairidx   := int(match_hands.pairs [len(match_hands.pairs )-1])
    threesidx := int(match_hands.threes[len(match_hands.threes)-1])
    player.hand.cards = append(cards[pairidx:pairidx+2], cards[threesidx:threesidx+3]...)
    assert(len(player.hand.cards) == 5, fmt.Sprintf("%d", len(player.hand.cards)))
    return
  }

  // flush search //
  //
  // NOTE: we search for the flush here to ease the straight flush logic
  have_flush, suit := got_flush(cards, player, false)

  // remove duplicate card (by number) for easy straight search
  unique_cards  := Cards{}

  if have_flush { // !! FIXME !!
  // check for possible straight flush suit
    cardmap := make(map[int]int) // key == num, val == suit
    for _, card := range cards {
      mappedsuit, found := cardmap[card.numvalue];
      if found && mappedsuit != suit && card.suit == suit {
        cardmap[card.numvalue] = card.suit
        assert(unique_cards[len(unique_cards)-1].numvalue == card.numvalue, "unique_cards problem")
        unique_cards[len(unique_cards)-1] = card // should _always_ be last card
      } else if !found {
        cardmap[card.numvalue] = card.suit
        unique_cards = append(unique_cards, card)
      }
    }
    assert((len(unique_cards) <= 7 && len(unique_cards) >= 3),
           fmt.Sprintf("impossible number of unique cards (%v)", len(unique_cards)))
  } else {
    cardmap := make(map[int]bool)
    for _, card := range cards {
      if _, val := cardmap[card.numvalue]; !val {
        cardmap[card.numvalue] = true
        unique_cards = append(unique_cards, card)
      }
    }
    assert((len(unique_cards) <= 7 && len(unique_cards) >= 1),
           "impossible number of unique cards")
  }

  if len(player.hand.cards) > 0 { panic ("b4 straight block\n")}
  // straight search //
  if len(unique_cards) >= 5 {
    unique_bestcard := len(unique_cards)
    iter := unique_bestcard - 4
    //fmt.Printf("iter %v len(uc) %d\n)", iter, len(unique_cards))
    for i := 1; i <= iter; i++ {
      if got_straight(&unique_cards, player, unique_bestcard-i, false) {
        assert(len(player.hand.cards) == 5, fmt.Sprintf("%d", len(player.hand.cards)))
        return
      }
    }
    if unique_cards[unique_bestcard-1].numvalue == C_ACE &&
       got_straight(&unique_cards, player, 4, true) {
      assert(len(player.hand.cards) == 5, fmt.Sprintf("%d", len(player.hand.cards)))
      return
    }
  }

  // threes search
  if match_hands.threes != nil {
    firstcard := int(match_hands.threes[len(match_hands.threes)-1])

    threeslice := make(Cards, 0, 3)
    threeslice  = append(threeslice, cards[firstcard:firstcard+3]...)

    kickers := top_cards(cards, 2, []int{cards[firstcard].numvalue})
    // order => [kickers][threes]
    kickers = append(kickers, threeslice...)

    player.hand.rank  = R_THREES
    player.hand.cards = kickers
    return
  }

  // two pair & pair search
  if match_hands.pairs != nil {
    if len(match_hands.pairs) > 1 {
      player.hand.rank   = R_2PAIR
      highpairidx := int(match_hands.pairs[len(match_hands.pairs)-1])
      lowpairidx  := int(match_hands.pairs[len(match_hands.pairs)-2])
      player.hand.cards = append(cards[lowpairidx:lowpairidx+2],
                                 cards[highpairidx:highpairidx+2]...)
      kicker := top_cards(cards, 1, []int{cards[highpairidx].numvalue,
                                           cards[lowpairidx ].numvalue})
      player.hand.cards = append(kicker, player.hand.cards...)
    } else {
      player.hand.rank = R_PAIR
      pairidx := match_hands.pairs[0]
      kickers := top_cards(cards, 3, []int{cards[pairidx].numvalue})
      player.hand.cards = append(kickers, cards[pairidx:pairidx+2]...)
    }
    return
  }

  // muck
  player.hand.rank   = R_HIGHCARD
  player.hand.cards = append(player.hand.cards, cards[bestcard-1],
                             cards[bestcard-2], cards[bestcard-3],
                             cards[bestcard-4], cards[bestcard-5])

  return
}

func cards_sort(cards *Cards) error {
  sort.Slice((*cards), func(i, j int) bool {
    return (*cards)[i].numvalue < (*cards)[j].numvalue
  })

  return nil
}

func card_num2str(card *Card) error {
  var name, suit, suit_full string
  switch card.numvalue {
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
    fmt.Printf("c: %s %d %d\n", card.name, card.numvalue, card.suit)
    return errors.New("card_num2str")
  }

  switch card.suit {
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

  card.name     = name + " "    + suit
  card.fullname = name + " of " + suit_full

  return nil
}

func assert(cond bool, msg string) {
  if !cond {
    panic(msg)
  }
}

func main() {
	deck := &Deck{}
  if err := deck.Init(); err != nil {
    return
  }

  table := &Table{ numplayers: 6 }
  if err := table.Init(deck); err != nil {
    return
  }

  deck.Shuffle()
  table.Deal()
  table.DoFlop()
  table.DoTurn()
  table.DoRiver()
  table.PrintSortedCommunity()
  table.BestHand()
}
