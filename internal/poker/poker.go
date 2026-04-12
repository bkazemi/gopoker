package poker

import (
	"fmt"

	//"net"

	//"io"

	"errors"
	"sync"

	//_ "net/http/pprof"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/rs/zerolog/log"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var printer *message.Printer

func init() {
	printer = message.NewPrinter(language.English)
}

type Chips uint64

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
	Ante     Chips    // current ante TODO allow both ante & blind modes
	Bet      Chips    // current bet

	Dealer     *PlayerNode // current dealer
	SmallBlind *PlayerNode // current small blind
	BigBlind   *PlayerNode // current big blind

	players       []*Player   // array of all seats at table
	activePlayers PlayerList  // list of all active players
	curPlayers    PlayerList  // list of actively betting players (no folders or all-ins)
	Winners       []*Player   // array of round winners
	curPlayer     *PlayerNode // keeps track of whose turn it is
	better        *Player     // last player to (re-)raise XXX currently unused
	NumPlayers    uint8       // number of current players
	NumSeats      uint8       // number of total possible players
	roundCount    uint64      // total number of rounds played

	WinInfo string // XXX tmp

	State        TableState // current status of table
	CommState    TableState // current status of community
	NumConnected uint64     // number of people (players+spectators) currently at table (online mode)

	Lock     TableLock // table admin option that restricts new connections
	Password string    // table password (optional)

	mtx sync.Mutex
}

// NOTE: the methods defined below are necessary so that
// other internal packages can access/modify private fields. said private fields
// cannot be simply made public because we send a Table struct to clients.

func (table *Table) SidePots() *SidePots {
	return &table.sidePots
}

func (table *Table) Players() *[]*Player {
	return &table.players
}

func (table *Table) ActivePlayers() *PlayerList {
	return &table.activePlayers
}

func (table *Table) CurPlayers() *PlayerList {
	return &table.curPlayers
}

func (table *Table) CurPlayer() *PlayerNode {
	return table.curPlayer
}

func (table *Table) SetCurPlayer(player *PlayerNode) {
	table.curPlayer = player
}

// IsCurPlayer reports whether p is the player whose turn is active. Uses
// Name comparison to match the convention used elsewhere (removePlayer,
// blind handling); pointer identity isn't guaranteed to line up across
// re-seats.
func (table *Table) IsCurPlayer(p *Player) bool {
	curP := table.CurPlayer()
	return curP != nil && p != nil && curP.Player.Name == p.Name
}

func (table *Table) Better() *Player {
	return table.better
}

func (table *Table) SetBetter(player *Player) {
	table.better = player
}

func (table *Table) validateNumSeats(numSeats uint8) error {
	if numSeats < 2 || numSeats > 7 {
		return errors.New("numSeats must be between 2 and 7")
	} else if numSeats < table.NumPlayers {
		return errors.New("numSeats must be greater than the current number of players")
	}

	return nil
}

func (table *Table) ValidateNumSeats(numSeats uint8) error {
	table.mtx.Lock()
	defer table.mtx.Unlock()

	return table.validateNumSeats(numSeats)
}

func (table *Table) SetNumSeats(numSeats uint8) error {
	table.mtx.Lock()
	defer table.mtx.Unlock()

	if err := table.validateNumSeats(numSeats); err != nil {
		return err
	}

	table.NumSeats = numSeats

	return nil
}

// XXX we should probably only have the poker package
// accessing the table lock, but for now i'm leaving it.
func (table *Table) Mtx() *sync.Mutex {
	return &table.mtx
}

func NewTable(deck *Deck, numSeats uint8, lock TableLock, password string, CPUPlayers []bool) (*Table, error) {
	if numSeats < 2 || numSeats > 7 {
		return nil, errors.New("numPlayers must be between 2 and 7")
	}

	players := make([]*Player, 0, numSeats)
	for i := uint8(0); i < numSeats; i++ {
		players = append(players, NewPlayer(fmt.Sprintf("p%d", i), CPUPlayers[i]))
	}

	table := &Table{
		deck: deck,
		Ante: 10, // TODO: add knob

		MainPot:  NewPot("mainpot", 0), // TODO: make .Players private
		sidePots: *NewSidePots(),

		players:       players,
		activePlayers: *NewPlayerList("activePlayers", nil),
		curPlayers:    *NewPlayerList("curPlayers", nil),

		Lock:     lock,
		Password: password,

		NumSeats: numSeats,
	}

	table.newCommunity()

	//table.curPlayers = table.players

	return table, nil
}

func (table *Table) Reset(player *Player) {
	table.mtx.Lock()
	defer table.mtx.Unlock()

	log.Info().Msg("resetting table")

	table.Ante = 10

	table.newCommunity()

	for _, node := range table.curPlayers.ToNodeArray() {
		if player == nil || player.Name != node.Player.Name {
			log.Debug().Str("player", node.Player.Name).Msg("cleared")
			node.Player.Clear()
			table.curPlayers.RemovePlayer(node.Player)
		} else { // XXX: not being found sometimes
			log.Debug().Str("player", node.Player.Name).Msg("skipped")
			table.curPlayers.SetHead(node)

			//player.NewCards()
			//player.Action.Action, player.Action.Amount = playerState.FirstAction, 0
		}
	}

	table.Winners, table.better = nil, nil

	if table.curPlayers.Len == 0 && player != nil {
		log.Debug().Str("player", player.Name).Msg("curPlayers was empty, adding winner")
		table.curPlayers.AddPlayer(player)
	} else {
		table.curPlayer = table.curPlayers.Head
	}

	// XXX
	for _, p := range table.players {
		if player == nil || p.Name != player.Name {
			log.Debug().Str("player", p.Name).Msg("XXX: second clear()")
			p.Clear()
		}
	}

	table.MainPot.Clear()
	table.sidePots.Clear()
	log.Debug().Msg("all pots cleared")

	table.Bet, table.NumPlayers, table.roundCount = 0, 0, 0

	if player != nil {
		log.Debug().Str("player", player.Name).Msg("clearing winner's action and cards")
		player.Action.Clear()
		player.NewCards()

		table.NumPlayers++
	}

	table.WinInfo = ""

	table.State = TableStateNotStarted

	table.Dealer = table.activePlayers.Head

	table.SmallBlind = nil
	table.BigBlind = nil
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

func (table *Table) DefaultPlayerNames() []string {
	names := make([]string, 0, len(table.players))

	for _, player := range table.players {
		names = append(names, player.defaultName)
	}

	return names
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

// NOTE: this should only be used to send information to frontends
//
//	because PlayerNode structs are not fully preserved
func (table *Table) PublicInfo() *Table {
	pubTable := *table

	if pubTable.Dealer != nil {
		pubTable.Dealer = &PlayerNode{
			Player: table.PublicPlayerInfo(*pubTable.Dealer.Player),
		}
	}
	if pubTable.SmallBlind != nil {
		pubTable.SmallBlind = &PlayerNode{
			Player: table.PublicPlayerInfo(*pubTable.SmallBlind.Player),
		}
	}
	if pubTable.BigBlind != nil {
		pubTable.BigBlind = &PlayerNode{
			Player: table.PublicPlayerInfo(*pubTable.BigBlind.Player),
		}
	}

	if pubTable.MainPot != nil {
		// deep copy MainPot.Players map
		pubTable.MainPot.Players = make(map[string]*Player)
		for name, player := range table.MainPot.Players {
			pubTable.MainPot.Players[name] = table.PublicPlayerInfo(*player)
		}
	}

	if pubTable.Winners != nil {
		// deep copy Winners slice
		pubTable.Winners = make([]*Player, len(pubTable.Winners))
		for idx, winner := range table.Winners {
			pubTable.Winners[idx] = table.PublicPlayerInfo(*winner)
		}
	}

	return &pubTable
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
		log.Debug().Str("player", p.Name).Str("action", p.ActionToString()).Msg("allInCount")
		if p.Action.Action == playerState.AllIn {
			allIns++
		}
	}

	return allIns
}

func (table *Table) BettingIsImpossible() bool {
	// only <= 1 player(s) has any chips left to bet
	return table.curPlayers.Len < 2
}

func (table *Table) getChipLeaders(includeAllIns bool) (Chips, Chips) {
	if table.curPlayers.Len < 2 {
		panic("BUG: Table.getChipLeaders() called with < 2 non-folded/allin players")
	}

	var (
		chipLeader       Chips
		chipLeaderName   string
		secondChipLeader Chips
	)

	var players []*Player
	if includeAllIns {
		players = table.GetNonFoldedPlayers()
	} else {
		players = table.curPlayers.ToPlayerArray()
	}

	for _, p := range players {
		log.Debug().
			Str("player", p.Name).
			Uint64("actionAmt", uint64(p.Action.Amount)).
			Msg("getChipLeaders")
	}

	for _, p := range players {
		blindRequiredBet := Chips(0)
		//if p.isABlind(table) {
		//  blindRequiredBet = p.Action.Amount
		//  printer.Printf("Table.getChipLeaders(): %s has blindRequiredBet %d\n", p.Name, blindRequiredBet)
		//}
		realChipCount := p.ChipCount + (p.Action.Amount - blindRequiredBet)
		if realChipCount > chipLeader {
			chipLeader = realChipCount
			chipLeaderName = p.Name
		}
	}

	for _, p := range players {
		if p.Name == chipLeaderName {
			continue
		}

		blindRequiredBet := Chips(0)
		//if p.isABlind(table) {
		//  blindRequiredBet = p.Action.Amount
		//  printer.Printf("Table.getChipLeaders(): %s has blindRequiredBet %d\n", p.Name, blindRequiredBet)
		//}

		realChipCount := p.ChipCount + (p.Action.Amount - blindRequiredBet)

		if realChipCount == chipLeader {
			// chipLeader and secondChipLeader had the same amount of chips
			return chipLeader, chipLeader
		}

		if realChipCount != chipLeader &&
			realChipCount > secondChipLeader {
			secondChipLeader = realChipCount
		}
	}

	if secondChipLeader == 0 { // all curPlayers have same chip count
		secondChipLeader = chipLeader
	}

	return chipLeader, secondChipLeader
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

	if player != nil {
		cc := player.ChipCountToString()
		defer func() {
			if cc == player.ChipCountToString() {
				log.Debug().Str("player", player.Name).Msg("chipcount unchanged")
			} else {
				log.Debug().
					Str("player", player.Name).
					Str("from", cc).
					Str("to", player.ChipCountToString()).
					Msg("chipcount changed")
			}
		}()
	}

	var blindRequiredBet Chips = 0

	isSmallBlindPreFlop := false

	if table.CommState == TableStatePreFlop &&
		table.State != TableStatePlayerRaised { // XXX mixed states...
		if table.SmallBlind != nil && player.Name == table.SmallBlind.Player.Name {
			isSmallBlindPreFlop = true
			blindRequiredBet = min(table.SmallBlind.Player.ChipCount, table.Ante/2)
		} else if table.BigBlind != nil && player.Name == table.BigBlind.Player.Name {
			blindRequiredBet = min(table.BigBlind.Player.ChipCount, table.Ante)
		}
	}

	if table.curPlayers.Len == 1 &&
		(action.Action == playerState.AllIn || action.Action == playerState.Bet) {
		return errors.New(printer.Sprintf("you must call the raise (%d chips) or fold", table.Bet))
	}

	if player.ChipCount == 0 && action.Action != playerState.AllIn {
		log.Debug().Str("player", player.Name).Msg("changing all-in bet to an allin action")
		action.Action = playerState.AllIn
	}

	switch action.Action {
	case playerState.AllIn:
		player.Action.Action = playerState.AllIn

		prevChips := player.Action.Amount
		log.Debug().Str("player", player.Name).Uint64("prevChips", uint64(prevChips)).Msg("allin")

		player.Action.Amount += prevChips
		player.ChipCount += prevChips

		if table.BettingIsImpossible() {
			log.Debug().Str("player", player.Name).Msg("allin: last player went all-in")
			player.Action.Amount = min(table.Bet, player.ChipCount)
		} else {
			chipLeaderCount, secondChipLeaderCount := table.getChipLeaders(true)
			log.Debug().
				Str("player", player.Name).
				Uint64("chipLeader", uint64(chipLeaderCount)).
				Uint64("2ndChipLeader", uint64(secondChipLeaderCount)).
				Msg("allin chip leaders")

			// NOTE: A chipleader can only bet what at least one other player can match.
			if player.ChipCount >= table.Bet && player.ChipCount == chipLeaderCount {
				player.Action.Amount = secondChipLeaderCount
			} else {
				player.Action.Amount = player.ChipCount
			}

			if player.Action.Amount > table.Bet {
				table.Bet = player.Action.Amount
				table.State = TableStatePlayerRaised
				table.better = player
				if table.curPlayers.Head.Player.Name != table.curPlayer.Player.Name {
					log.Debug().
						Str("player", player.Name).
						Str("newHead", table.curPlayer.Player.Name).
						Msg("allin: setting curPlayers head")
					table.curPlayers.SetHead(table.curPlayer) // NOTE: the new better always
					// becomes the head of the table
				}
			}
		}

		/*if table.sidePots.IsEmpty() {
		  if prevChips > 0 {
		    fmt.Printf("Table.PlayerAction(): allin: removing prevChips from mainpot\n")
		    table.MainPot.Total -= prevChips
		  }*/

		table.handleSidePots(player, prevChips, 0)

		player.ChipCount -= player.Action.Amount
	case playerState.Bet:
		prevChips := player.Action.Amount
		log.Debug().Str("player", player.Name).Uint64("prevChips", uint64(prevChips)).Msg("bet")

		if action.Amount < table.Ante {
			return errors.New(printer.Sprintf("bet must be greater than the ante (%d chips)", table.Ante))
		} else if action.Amount <= table.Bet {
			return errors.New(printer.Sprintf("bet must be greater than the current bet (%d chips)", table.Bet))
		} else if action.Amount > player.ChipCount+prevChips {
			return errors.New("not enough chips")
		}

		chipLeaderCount, secondChipLeaderCount := table.getChipLeaders(true)
		log.Debug().
			Str("player", player.Name).
			Uint64("chipLeader", uint64(chipLeaderCount)).
			Uint64("2ndChipLeader", uint64(secondChipLeaderCount)).
			Msg("bet chip leaders")

		log.Debug().
			Str("player", player.Name).
			Uint64("prevChips", uint64(prevChips)).
			Msg("bet: adding prevChips")
		player.ChipCount += prevChips

		// NOTE: A chipleader can only bet what at least one other player can match.
		if player.ChipCount == chipLeaderCount {
			player.Action.Amount = min(action.Amount, secondChipLeaderCount)
		} else {
			player.Action.Amount = action.Amount
		}

		if action.Amount == player.ChipCount {
			player.Action.Action = playerState.AllIn
		} else {
			player.Action.Action = playerState.Bet
		}

		if player.Action.Action == playerState.AllIn || !table.sidePots.IsEmpty() {
			table.handleSidePots(player, prevChips, 0)
		} else {
			table.MainPot.Total += player.Action.Amount - prevChips
		}

		player.ChipCount -= player.Action.Amount

		table.Bet = player.Action.Amount

		log.Debug().
			Str("player", player.Name).
			Str("newHead", table.curPlayer.Player.Name).
			Msg("bet: setting curPlayers head")
		table.curPlayers.SetHead(table.curPlayer)
		table.better = player
		table.State = TableStatePlayerRaised
	case playerState.Call:
		if table.State != TableStatePlayerRaised && !isSmallBlindPreFlop {
			return errors.New("nothing to call")
		}

		if (table.SmallBlind != nil && player.Name == table.SmallBlind.Player.Name) ||
			(table.BigBlind != nil && player.Name == table.BigBlind.Player.Name) {
			log.Debug().
				Str("player", player.Name).
				Uint64("actionAmt", uint64(player.Action.Amount)).
				Msg("call: blind")
		}

		prevChips := player.Action.Amount
		log.Debug().Str("player", player.Name).Uint64("prevChips", uint64(prevChips)).Msg("call")

		player.ChipCount += prevChips

		// delta of bet & curPlayer's last bet
		betDiff := table.Bet - player.Action.Amount

		log.Debug().Str("player", player.Name).Uint64("betDiff", uint64(betDiff)).Msg("call: betDiff")

		if table.Bet >= player.ChipCount {
			player.Action.Action = playerState.AllIn
			player.Action.Amount = player.ChipCount

			table.handleSidePots(player, prevChips, 0)

			player.ChipCount = 0
		} else {
			player.Action.Action = playerState.Call
			player.Action.Amount = table.Bet

			if !table.sidePots.IsEmpty() {
				table.handleSidePots(player, prevChips, betDiff)
			} else {
				table.MainPot.Total += betDiff
			}
			player.ChipCount -= player.Action.Amount
		}
	case playerState.Check:
		if table.State == TableStatePlayerRaised {
			return errors.New(printer.Sprintf("you must call the raise (%d chips)", table.Bet))
		}

		if isSmallBlindPreFlop {
			return errors.New(printer.Sprintf("you must call the big blind (+%d chips)", blindRequiredBet))
		}

		if player.ChipCount == 0 { // big blind had a chipcount <= ante
			player.Action.Action = playerState.AllIn
		} else {
			player.Action.Action = playerState.Check
		}
	case playerState.Fold:
		player.Action.Action = playerState.Fold
	default:
		return errors.New(fmt.Sprintf("BUG: invalid player action: %b", action.Action))
	}

	table.SetNextPlayerTurn()

	return nil
}
