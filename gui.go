// game graphical user interface

package main

import (
  "io/ioutil"
  "path/filepath"
  "strconv"
  "regexp"

  "time"

  _ "image/png"

  "github.com/hajimehoshi/ebiten/v2"
  "github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
  screenWidth, screenHeight = 1280, 720
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

  // suit offsets in deck slice
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

type Game struct {
  table     *Table
  cardImgMap cardImgMap_t
  mode       Mode
}

type Mode int

const (
  M_INIT   Mode = iota
  M_BET
  M_DOFLOP
  M_DOTURN
  M_DORIVER
  M_BESTHAND
  M_GAMEOVER
)

func (g *Game) Update() error {
  switch g.mode {
  case M_INIT:
    g.table.deck.Shuffle()
    g.table.Deal()
    g.mode = M_DOFLOP
    return nil
  case M_DOFLOP:
    g.table.DoFlop()
    g.mode = M_DOTURN
    return nil
  case M_DOTURN:
    g.table.DoTurn()
    g.mode = M_DORIVER
  case M_DORIVER:
    g.table.DoRiver()
    g.mode = M_BESTHAND
  case M_BESTHAND:
    g.table.PrintSortedCommunity()
    //g.table.BestHand()
    g.mode = M_GAMEOVER
  case M_GAMEOVER:
    // TODO
    time.Sleep(100 * time.Millisecond)
  }

  return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
  if len(g.table.Community) == 0 {
    return
  }
  com1Op := &ebiten.DrawImageOptions{}
  com1Op.GeoM.Translate(0, 3)
  screen.DrawImage(g.cardImgMap[g.table.Community[0]].img, com1Op)
  com2Op := &ebiten.DrawImageOptions{}
  com2Op.GeoM.Translate(150, 3)
  screen.DrawImage(g.cardImgMap[g.table.Community[1]].img, com2Op)
  com3Op := &ebiten.DrawImageOptions{}
  com3Op.GeoM.Translate(300, 3)
  screen.DrawImage(g.cardImgMap[g.table.Community[2]].img, com3Op)
  if len(g.table.Community) > 3 {
    com4Op := &ebiten.DrawImageOptions{}
    com4Op.GeoM.Translate(450, 3)
    screen.DrawImage(g.cardImgMap[g.table.Community[3]].img, com4Op)
  }
  if len(g.table.Community) > 4 {
    com5Op := &ebiten.DrawImageOptions{}
    com5Op.GeoM.Translate(600, 3)
    screen.DrawImage(g.cardImgMap[g.table.Community[4]].img, com5Op)
  }

  return
}

func (g *Game) Layout(ow, oh int) (int, int) {
  return screenWidth, screenHeight
}

func gui_run(table *Table) error {
  cardImgMap, err := init_cards(table.deck.cards)
  if err != nil {
    return err
  }

  game := &Game{ table: table, cardImgMap: cardImgMap }
  ebiten.SetWindowSize(screenWidth, screenHeight)
  ebiten.SetWindowTitle("gopoker")

  if err := ebiten.RunGame(game); err != nil {
    return err
  }

  return nil
}
