package poker

import math_rand "math/rand"

type Deck struct {
  pos   uint
  cards Cards
  size  int
}

func NewDeck() *Deck {
  deck := &Deck{
    size: 52, // 52 cards in a poker deck
  }
  deck.cards = make(Cards, deck.size, deck.size)

  for suit := SuitClub; suit <= SuitSpade; suit <<= 1 {
    for c_num := CardTwo; c_num <= CardAce; c_num++ {
      curCard := &Card{Suit: suit, NumValue: CardVal(c_num)}
      if err := cardNumToString(curCard); err != nil {
        panic(err)
      }

      deck.cards[deck.pos] = curCard
      deck.pos++
    }
  }

  deck.pos = 0

  return deck
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
