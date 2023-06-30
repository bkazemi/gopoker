package poker

import (
	"errors"
	"fmt"
	"sort"
)

// ranks
type Rank int8
const (
  RankMuck  Rank = iota - 1
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
type Suit uint8
const (
  SuitClub Suit = 1 << iota
  SuitDiamond
  SuitHeart
  SuitSpade
)

type CardVal uint8
type Card struct {
  Name     string
  FullName string
  Suit     Suit
  NumValue CardVal // numeric value of card
}

type Cards []*Card

type Hole struct {
  IsSuited         bool
  IsPair           bool
  Suit             Suit
  CombinedNumValue uint16
  Cards            Cards
}

func (hole *Hole) FillHoleInfo() {
  var (
    cardOne = hole.Cards[0]
    cardTwo = hole.Cards[1]
  )

  if cardOne.NumValue == cardTwo.NumValue {
    hole.IsPair = true
  }

  if cardOne.Suit == cardTwo.Suit {
    hole.IsSuited = true
    hole.Suit = cardOne.Suit | cardTwo.Suit
  }

  hole.CombinedNumValue = uint16(cardOne.NumValue + cardTwo.NumValue)
}

type Hand struct {
  Rank   Rank
  Kicker uint8
  Cards  Cards
}

func (hand *Hand) RankName() string {
  rankNameMap := map[Rank]string{
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
    RankRoyalFlush:    "royal flush",
  }

  if rankName, ok := rankNameMap[hand.Rank]; ok {
    return rankName
  }

  panic("Hand.RankName(): BUG")
}

func cardsSort(cards *Cards) error {
  sort.Slice((*cards), func(i, j int) bool {
    return (*cards)[i].NumValue < (*cards)[j].NumValue
  })

  return nil
}

func reverseCards(cards Cards) Cards {
  reversed := make(Cards, len(cards))

  for i, j := len(cards) - 1, 0; i >= 0; i, j = i - 1, j + 1 {
      reversed[j] = cards[i]
  }

  return reversed
}

func cardNumToString(card *Card) error {
  cardNumStringMap := map[CardVal]string{
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
    fmt.Println("cardNumToString(): BUG: couldn't find cardNum name")
    fmt.Printf("c: %s %d %d\n", card.Name, card.NumValue, card.Suit)
    return errors.New("cardNumToString")
  }

  cardSuitStringMap := map[Suit][]string{
    SuitClub:    {"♣", "clubs"},
    SuitDiamond: {"♦", "diamonds"},
    SuitHeart:   {"♥", "hearts"},
    SuitSpade:   {"♠", "spades"},
  }

  suitName := cardSuitStringMap[card.Suit]
  if suitName == nil {
    // TODO: fix redundancy.
    fmt.Println("cardNumToString(): BUG: couldn't find suitName")
    fmt.Printf("c: %s %d %d\n", card.Name, card.NumValue, card.Suit)
    return errors.New("cardNumToString")
  }

  suit, suit_full := suitName[0], suitName[1]

  card.Name = name + " " + suit
  card.FullName = name + " of " + suit_full

  return nil
}
