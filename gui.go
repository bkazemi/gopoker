// game graphical user interface

package main

import (
  "fmt"
  "io/ioutil"
  "path/filepath"
  "strconv"
  "regexp"

  _ "image/png"

  "github.com/hajimehoshi/ebiten/v2"
  "github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
  screenWidth, screenHeight = 640, 480
)

// all items drawn to the screen
type screenObject struct {
  img       *ebiten.Image
  xPos, yPos float64
}

type cardImgMap_t map[*Card]*screenObject

// must be called before deck is shuffled to get proper offsets
func init_cards(cards Cards) (cardImgMap_t, error) {
  cardImgs, err := ioutil.ReadDir(filepath.Join("assets/sprites/cards"))
  if err != nil {
    return nil, err
  }

  cardImgMap := make(cardImgMap_t)

  clubPos    := S_CLUB     + 0
  diamondPos := clubPos    + (C_ACE - 1)
  heartPos   := diamondPos + (C_ACE - 1)
  spadePos   := heartPos   + (C_ACE - 1)

  suitRegex := regexp.MustCompile(`(Clubs|Diamonds|Hearts|Spades)`)
  numRegex  := regexp.MustCompile(`([\d]+|[JKQA])$`)

  to_num := func (n string) int {
    switch n {
    // NOTE: because the first card is a two, that is what we subtract to get
    //       the index
    case "J":
      return C_JACK  - 2
    case "Q":
      return C_QUEEN - 2
    case "K":
      return C_KING  - 2
    case "A":
      return C_ACE   - 2
    default:
      parsedNum, err := strconv.ParseInt(n, 10, 8)
      if err != nil {
        panic(err)
      }
      return int(parsedNum) - 2
    }
  }

  for _, cardImg := range cardImgs {
    name := cardImg.Name()[0:len(cardImg.Name())-4] // remove xxx.png
    suit := suitRegex.FindString(name)
    num  := numRegex.FindString(name)

    var card *Card
    switch suit {
    case "Clubs":
      card = cards[clubPos    + to_num(num)]
    case "Diamonds":
      card = cards[diamondPos + to_num(num)]
    case "Hearts":
      card = cards[heartPos   + to_num(num)]
    case "Spades":
      card = cards[spadePos   + to_num(num)]
    }

    if card != nil {
      img, _, err := ebitenutil.NewImageFromFile(filepath.Join("assets/sprites/cards", cardImg.Name()))
      if err != nil {
        return cardImgMap_t{}, err
      }
      cardImgMap[card] = &screenObject{img, -1, -1}
    }
  }

  return cardImgMap, nil
}

// called in main()
func gui_init(table *Table) error {
  cardImgMap, err := init_cards(table.deck.cards)
  if err != nil {
    return err
  }

  for k, v := range cardImgMap {
    fmt.Printf("%v -> %v\n", k, v)
  }

  return nil
}
