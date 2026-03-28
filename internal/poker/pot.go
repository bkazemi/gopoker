package poker

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

type Pot struct {
  Name     string // XXX tmp for debugging
  Bet      Chips
  Total    Chips
  Players  map[string]*Player
  IsClosed bool
  WinInfo  string
}

func NewPot(name string, bet Chips) *Pot {
  if name == "" {
    name = "unnamed pot"
  }

  pot := &Pot{
    Name: name,
    Bet: bet,
    Players: make(map[string]*Player),
  }

  return pot
}

func (pot *Pot) WithPlayers(players map[string]*Player) *Pot {
  for name, p := range players {
    pot.Players[name] = p
  }

  return pot
}

func (pot *Pot) HasPlayer(player *Player) bool {
  return pot.Players[player.Name] != nil
}

func (pot *Pot) AddPlayer(player *Player) {
  log.Debug().Str("pot", pot.Name).Uint64("bet", uint64(pot.Bet)).Str("player", player.Name).Msg("added player")
  pot.Players[player.Name] = player
}

func (pot *Pot) AddPlayers(playerMap map[string]*Player) {
  for name, player := range playerMap {
    log.Debug().Str("pot", pot.Name).Uint64("bet", uint64(pot.Bet)).Str("player", name).Msg("added player")
    pot.Players[name] = player
  }
}

func (pot *Pot) RemovePlayer(player *Player) {
  if player == nil {
    log.Debug().Str("pot", pot.Name).Uint64("bet", uint64(pot.Bet)).Msg("clearing pot")
    pot.Players = make(map[string]*Player)
  } else {
    log.Debug().Str("pot", pot.Name).Uint64("bet", uint64(pot.Bet)).Str("player", player.Name).Msg("removed player")
    delete(pot.Players, player.Name)
  }
}

func (pot *Pot) PlayerInfo() string {
  numPlayers := len(pot.Players)

  if numPlayers == 0 {
    return "#p 0"
  }

  playerNames := "["
  for playerName := range pot.Players {
    playerNames += playerName + ", "
  }
  playerNames += "]"

  return printer.Sprintf("#p %d %s", numPlayers, playerNames)
}

func (pot *Pot) Clear() {
  defer func() {
    log.Debug().Str("pot", pot.Name).Msg("cleared")
  }()

  pot.Players = make(map[string]*Player)
  pot.Bet = 0
  pot.Total = 0
  pot.IsClosed = false
  pot.WinInfo = ""
}

type SidePot struct {
  *Pot // XXX
  MustCall *Pot
}

// XXX: mixed init constructs
func NewSidePot(bet Chips) *SidePot {
  name := "unknown sidepot"

  sidePot := &SidePot{
    Pot: NewPot(name, bet),
  }

  return sidePot
}

func (sidePot *SidePot) WithName(name string) *SidePot {
  sidePot.Name = name

  return sidePot
}

func (sidePot *SidePot) WithPlayers(players map[string]*Player) *SidePot {
  for name, p := range players {
    sidePot.Players[name] = p
  }

  return sidePot
}

func (sidePot *SidePot) WithPlayer(player *Player) *SidePot {
  sidePot.Name = player.Name + " sidePot"
  sidePot.Players[player.Name] = player

  return sidePot
}

func (sidePot *SidePot) WithMustCall(mustCall *Pot) *SidePot {
  sidePot.MustCall = mustCall

  return sidePot
}

func (sidePot *SidePot) Calculate(prevBet Chips) {
  log.Debug().Str("sidePot", sidePot.Name).Uint64("prevBet", uint64(prevBet)).Msg("calculating")

  defer func() {
    log.Debug().Str("sidePot", sidePot.Name).Uint64("total", uint64(sidePot.Total)).Msg("calculated")
  }()

  // MustCall struct contains players who folded on
  // an allin re-raise
  if sidePot.MustCall != nil {
    log.Debug().Str("mustCall", sidePot.MustCall.Name).Str("playerInfo", sidePot.MustCall.PlayerInfo()).Msg("calculating")
    mustCallChips := sidePot.MustCall.Bet * Chips(len(sidePot.MustCall.Players))
    log.Debug().Str("mustCall", sidePot.MustCall.Name).Uint64("total", uint64(mustCallChips)).Msg("adding mustCall to sidePot")
    sidePot.Total += mustCallChips
  }

  sidePot.Total += (sidePot.Bet - prevBet) * Chips(len(sidePot.Players))
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

func (arr *SidePotArray) GetPotsStartingAt(idx int) []*SidePot {
  if idx < 0 || idx > len(arr.Pots)-1 {
    return []*SidePot{}
  }

  return arr.Pots[idx:]
}

func (arr *SidePotArray) GetLargest() *SidePot {
  openSidePots := arr.GetOpenPots()

  if len(openSidePots) == 0 {
    return nil
  }

  return openSidePots[len(openSidePots)-1]
}

func (arr *SidePotArray) CloseAll() {
  for _, sidePot := range arr.GetOpenPots() {
    log.Debug().Str("sidePot", sidePot.Name).Msg("closing")
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

func NewSidePots() *SidePots {
  return &SidePots{
    AllInPots: SidePotArray{
      Pots: make([]*SidePot, 0),
    },
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
  sidePots.AllInPots = SidePotArray{
    Pots: make([]*SidePot, 0),
  }
  sidePots.BettingPot = nil
}

func (sidePots *SidePots) Print() {
  for _, sidePot := range sidePots.AllInPots.Pots {
    log.Debug().Str("sidePot", sidePot.Name).Uint64("bet", uint64(sidePot.Bet)).Uint64("pot", uint64(sidePot.Total)).Bool("closed", sidePot.IsClosed).Str("playerInfo", sidePot.PlayerInfo()).Msg("sidepot state")
  }

  if sidePot := sidePots.BettingPot; sidePot != nil {
    log.Debug().Str("sidePot", sidePot.Name).Uint64("bet", uint64(sidePot.Bet)).Uint64("pot", uint64(sidePot.Total)).Bool("closed", sidePot.IsClosed).Str("playerInfo", sidePot.PlayerInfo()).Msg("sidepot state")
  }
}
