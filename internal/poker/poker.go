package poker

import (
	"fmt"
	"maps"
	"slices"

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

func (table *Table) GetSeat(_pos uint8) *Player {
	log.Debug().Uint8("pos", _pos).Msg("GetSeat")
	// treat 0 index as a call to GetOpenSeat()
	// for easier integration with existing codebase
	if _pos == 0 {
		return table.GetOpenSeat()
	}

	table.mtx.Lock()
	defer table.mtx.Unlock()

	pos := int(_pos) - 1

	if pos > len(table.players)-1 || !table.players[pos].IsVacant {
		log.Warn().Int("pos", pos).Msg("requested OOB or occupied seat")
		return nil
	}

	seat := table.players[pos]
	seat.IsVacant = false
	seat.TablePos = uint(pos)
	table.NumPlayers++

	return seat
}

func (table *Table) GetOpenSeat() *Player {
	table.mtx.Lock()
	defer table.mtx.Unlock()

	if table.GetNumOpenSeats() == 0 {
		return nil
	}

	for i, seat := range table.players {
		if seat.IsVacant {
			seat.IsVacant = false
			seat.TablePos = uint(i)
			table.NumPlayers++

			return seat
		}
	}

	return nil
}

func (table *Table) GetOccupiedSeats() []*Player {
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
			seat.Action.Action != playerState.MidroundAddition {
			seats = append(seats, seat)
		}
	}

	return seats
}

func (table *Table) GetNumOpenSeats() uint8 {
	return table.NumSeats - table.NumPlayers
}

func (table *Table) addNewPlayers() {
	for _, player := range table.activePlayers.ToPlayerArray() {
		if player.Action.Action == playerState.MidroundAddition {
			log.Debug().Str("player", player.Name).Msg("adding new player")
			player.Action.Action = playerState.FirstAction
		}
	}
}

func (table *Table) GetEliminatedPlayers() []*Player {
	table.mtx.Lock()
	defer table.mtx.Unlock()

	ret := make([]*Player, 0)

	for _, player := range table.activePlayers.ToPlayerArray() {
		if player.ChipCount == 0 {
			ret = append(ret, player)
		}
	}

	if uint8(len(ret)) == table.NumPlayers-1 {
		table.State = TableStateGameOver
	} else {
		log.Debug().
			Uint("lenRet", uint(len(ret))).
			Uint8("np-1", table.NumPlayers-1).
			Msg("GetEliminatedPlayers")
	}

	names := make([]string, 0, len(ret))
	for _, p := range ret {
		names = append(names, p.Name)
	}
	log.Debug().Strs("eliminated", names).Msg("GetEliminatedPlayers")

	return ret
}

// resets the active players list head to
// Bb+1 pre-flop
// Sb post-flop
func (table *Table) ReorderPlayers() {
	if table.State == TableStateNewRound ||
		table.State == TableStatePreFlop {
		table.activePlayers.SetHead(table.BigBlind.Next())
		table.curPlayers.SetHead(table.curPlayers.GetPlayerNode(table.BigBlind.Next().Player))
		Assert(table.curPlayers.Head != nil,
			"Table.ReorderPlayers(): couldn't find Bb+1 player node")
		log.Debug().Str("curPlayersHead", table.curPlayers.Head.Player.Name).Msg("curPlayers head now")
	} else { // post-flop
		smallBlindNode := table.SmallBlind
		if smallBlindNode == nil { // smallblind left mid game
			if table.Dealer != nil {
				smallBlindNode = table.Dealer.Next()
			} else if table.BigBlind != nil {
				smallBlindNode = table.activePlayers.Head
				// definitely considering doubly linked lists now *sigh*
				for smallBlindNode.Next().Player.Name != table.BigBlind.Player.Name {
					smallBlindNode = smallBlindNode.Next()
				}
			} else {
				log.Warn().Msg("dealer, Sb & Bb all left mid round")
				table.handleOrphanedSeats()
				smallBlindNode = table.SmallBlind
			}
			log.Debug().Str("curPlayer", smallBlindNode.Player.Name).Msg("smallblind left mid round")
		}
		smallBlindNode = table.curPlayers.GetPlayerNode(smallBlindNode.Player)
		if smallBlindNode == nil {
			// small-blind folded or is all in so we need to search activePlayers for next actively betting player
			smallBlindNode = table.SmallBlind.Next()
			for !smallBlindNode.Player.canBet() {
				smallBlindNode = smallBlindNode.Next()
			}

			smallBlindNode = table.curPlayers.GetPlayerNode(smallBlindNode.Player)

			Assert(smallBlindNode != nil, "Table.ReorderPlayers(): couldn't find a nonfolded player after Sb")

			log.Debug().
				Str("smallBlind", table.SmallBlind.Player.Name).
				Str("curPlayer", smallBlindNode.Player.Name).
				Msg("smallBlind not active")
		}
		table.curPlayers.SetHead(smallBlindNode) // small blind (or next active player)
		// is always first better after pre-flop
	}

	table.curPlayer = table.curPlayers.Head
}

func (table *Table) handleOrphanedSeats() {
	// TODO: this is the corner case where D, Sb & Bb all leave mid-game. need to
	//       find a way to keep track of dealer pos to rotate properly.
	//
	//       considering making lists doubly linked.
	if table.Dealer == nil && table.SmallBlind == nil && table.BigBlind == nil {
		log.Warn().Msg("D, Sb & Bb all nil, resetting to activePlayers head")
		table.Dealer = table.activePlayers.Head
		table.SmallBlind = table.Dealer.Next()
		table.BigBlind = table.SmallBlind.Next()
	}
	if table.Dealer == nil && table.SmallBlind == nil { // (bigBlind != nil)
		var newDealerNode *PlayerNode
		for i, n := 0, table.activePlayers.Head; i < table.activePlayers.Len; i++ {
			if n.Next().Next().Player.Name == table.BigBlind.Player.Name {
				newDealerNode = n
				break
			}
			n = n.Next()
		}

		Assert(newDealerNode != nil, "Table.handleOrphanedSeats(): newDealerNode == nil")

		log.Debug().Str("dealer", newDealerNode.Player.Name).Msg("setting dealer")

		table.Dealer = newDealerNode
		table.SmallBlind = table.Dealer.Next()
		table.BigBlind = table.SmallBlind.Next()
	}

	if table.Dealer == nil {
		var newDealerNode *PlayerNode
		for i, n := 0, table.activePlayers.Head; i < table.activePlayers.Len; i++ {
			if n.Next().Player.Name == table.SmallBlind.Player.Name {
				newDealerNode = n
				break
			}
			n = n.Next()
		}

		Assert(newDealerNode != nil, "Table.handleOrphanedSeats(): newDealerNode == nil")

		log.Debug().Str("dealer", newDealerNode.Player.Name).Msg("setting dealer")

		table.Dealer = newDealerNode
	}

	if table.SmallBlind == nil {
		table.SmallBlind = table.Dealer.Next()
		log.Debug().Str("smallBlind", table.SmallBlind.Player.Name).Msg("setting smallblind")
	}

	if table.BigBlind == nil {
		table.BigBlind = table.SmallBlind.Next()
		log.Debug().Str("bigBlind", table.BigBlind.Player.Name).Msg("setting bigblind")
	}
}

// rotates the dealer and blinds
func (table *Table) rotatePlayers() {
	if table.State == TableStateNotStarted || table.activePlayers.Len < 2 {
		return
	}

	if table.Dealer == nil || table.SmallBlind == nil || table.BigBlind == nil {
		table.handleOrphanedSeats()
	}

	log.Debug().
		Str("dealer", table.Dealer.Player.Name).
		Str("smallBlind", table.SmallBlind.Player.Name).
		Str("bigBlind", table.BigBlind.Player.Name).
		Msg("before")

	Panic := &Panic{}

	defer Panic.IfNoPanic(func() {
		log.Debug().
			Str("dealer", table.Dealer.Player.Name).
			Str("smallBlind", table.SmallBlind.Player.Name).
			Str("bigBlind", table.BigBlind.Player.Name).
			Msg("after")

		table.ReorderPlayers()
	})

	if table.BigBlind.Next().Player.Name == table.Dealer.Player.Name {
		table.Dealer = table.BigBlind
	} else {
		table.Dealer = table.Dealer.Next()
	}
	table.SmallBlind = table.Dealer.Next()
	table.BigBlind = table.SmallBlind.Next()
}

func (table *Table) SetNextPlayerTurn() {
	log.Debug().Str("curPlayer", table.curPlayer.Player.Name).Msg("SetNextPlayerTurn")
	if table.State == TableStateNotStarted {
		return
	}

	table.mtx.Lock()
	defer table.mtx.Unlock()

	Panic := &Panic{}

	thisPlayer := table.curPlayer // save in case we need to remove from curPlayers list

	defer Panic.IfNoPanic(func() {
		if table.State == TableStateDoneBetting {
			table.better = nil
			table.calculateSidePotTotals() // TODO: move me
			table.closeSidePots()
		}

		if thisPlayer.Player.Action.Action == playerState.AllIn {
			nextNode := table.curPlayers.RemovePlayer(thisPlayer.Player)
			if nextNode != nil {
				log.Debug().
					Str("removed", thisPlayer.Player.Name).
					Str("headWas", table.curPlayers.Head.Player.Name).
					Msg("allIn removal")
				table.curPlayer = nextNode
				log.Debug().Str("headNow", table.curPlayers.Head.Player.Name).Msg("curPlayers head updated")
			}
		}

		log.Debug().Str("newCurPlayer", table.curPlayer.Player.Name).Msg("SetNextPlayerTurn")
		table.curPlayers.ToPlayerArray()
	})

	if table.curPlayers.Len == 1 {
		log.Debug().Msg("curPlayers.Len == 1")
		if table.allInCount() == 0 ||
			(table.allInCount() == 1 &&
				thisPlayer.Player.Action.Action == playerState.Fold) { // win by folds
			log.Debug().Msg("allInCount == 0 || (allInCount == 1 && curPlayer folded)")
			table.State = TableStateRoundOver // XXX
		} else {
			table.State = TableStateDoneBetting
		}

		return
	}

	if thisPlayer.Player.Action.Action == playerState.Fold {
		nextNode := table.curPlayers.RemovePlayer(thisPlayer.Player)
		if nextNode != nil {
			log.Debug().
				Str("removed", thisPlayer.Player.Name).
				Str("headWas", table.curPlayers.Head.Player.Name).
				Msg("fold removal")
			table.curPlayer = nextNode
			log.Debug().
				Str("headNow", table.curPlayers.Head.Player.Name).
				Msg("curPlayers head updated after fold")
		}
	} else {
		table.curPlayer = thisPlayer.Next()
	}

	if table.curPlayers.Len == 1 && table.allInCount() == 0 {
		log.Debug().Msg("curPlayers.Len == 1 with allInCount of 0 after fold")
		table.State = TableStateRoundOver // XXX

		return
	} else if thisPlayer.Next() == table.curPlayers.Head &&
		thisPlayer.Next().Player.Action.Action != playerState.FirstAction {
		/*((table.State == TableStatePlayerRaised &&
		  table.better.Name == table.curPlayers.Head.Player.Name)
		  (table.State != TableStatePlayerRaised &&
		   table.curPlayers.Head.Player.Action.Action != playerAction.FirstAction)) {*/
		// NOTE: curPlayers head gets shifted with allins / folds so we check for
		//       firstaction, /*however this doesn't work post-flop so we check
		//       the table better as well*/ <- I've opted to reset the action before each round for now
		log.Debug().Str("lastPlayer", thisPlayer.Player.Name).Msg("last player didn't raise")
		log.Debug().
			Str("curPlayersHead", table.curPlayers.Head.Player.Name).
			Str("curPlayerNext", table.curPlayer.Next().Player.Name).
			Msg("done betting")

		table.State = TableStateDoneBetting
	} else {
		//table.curPlayer = table.curPlayer.Next()
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
	cards := ""
	for _, card := range table.Community {
		cards += fmt.Sprintf("[%s] ", card.Name)
	}
	log.Debug().Str("cards", cards).Msg("PrintCommunity")
}

func (table *Table) PrintSortedCommunity() {
	cards := ""
	for _, card := range table._comsorted {
		cards += fmt.Sprintf(" [%s]", card.Name)
	}
	log.Debug().Str("cards", cards).Msg("PrintSortedCommunity")
}

// sort community cards by number
func (table *Table) SortCommunity() {
	cardsSort(&table._comsorted)
}

func (table *Table) NextCommunityAction() {
	switch table.CommState {
	case TableStatePreFlop:
		table.DoFlop()

		table.CommState = TableStateFlop
		if !table.BettingIsImpossible() { // else all players went all in preflop
			// and we are in the all-in loop
			table.ReorderPlayers()
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
		panic("BUG: Table.NextCommunityAction(): invalid community state")
	}

	table.State = TableStateRounds
}

func (table *Table) NextTableAction() {
	switch table.State {
	case TableStateNotStarted:
		if table.Dealer == nil || table.SmallBlind == nil || table.BigBlind == nil {
			table.handleOrphanedSeats()
		}

		table.Bet = table.Ante

		table.SmallBlind.Player.Action.Amount = min(table.Ante/2,
			table.SmallBlind.Player.ChipCount)
		table.BigBlind.Player.Action.Amount = min(table.Ante,
			table.BigBlind.Player.ChipCount)

		table.SmallBlind.Player.ChipCount -= table.SmallBlind.Player.Action.Amount
		table.BigBlind.Player.ChipCount -= table.BigBlind.Player.Action.Amount

		table.MainPot.Total = table.SmallBlind.Player.Action.Amount + table.BigBlind.Player.Action.Amount

		table.Deal()

		table.CommState = TableStatePreFlop

		table.ReorderPlayers() // NOTE: need to call this to properly set curPlayer
	case TableStateNewRound:
		table.rotatePlayers()

		table.Bet = table.Ante

		table.SmallBlind.Player.Action.Amount = min(table.Ante/2,
			table.SmallBlind.Player.ChipCount)
		table.BigBlind.Player.Action.Amount = min(table.Ante,
			table.BigBlind.Player.ChipCount)

		table.SmallBlind.Player.ChipCount -= table.SmallBlind.Player.Action.Amount
		table.BigBlind.Player.ChipCount -= table.BigBlind.Player.Action.Amount

		table.MainPot.Total = table.SmallBlind.Player.Action.Amount + table.BigBlind.Player.Action.Amount

		if table.SmallBlind.Player.ChipCount == 0 {
			table.SmallBlind.Player.Action.Action = playerState.AllIn
		}
		if table.BigBlind.Player.ChipCount == 0 {
			table.BigBlind.Player.Action.Action = playerState.AllIn
		}

		table.MainPot.Total = table.SmallBlind.Player.Action.Amount + table.BigBlind.Player.Action.Amount

		table.Deal()

		table.CommState = TableStatePreFlop
	case TableStateGameOver:
		log.Info().Msg("game over!")

	default:
		log.Error().Str("state", table.TableStateToString()).Msg("BUG: called with improper state")
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

func (table *Table) GetNonFoldedPlayers() []*Player {
	players := make([]*Player, 0)

	for _, player := range table.getActiveSeats() {
		if player.Action.Action != playerState.Fold {
			players = append(players, player)
		}
	}

	Assert(len(players) != 0, "Table.getNonFoldedPlayers(): BUG: len(players) == 0")

	return players
}

func (table *Table) NewRound() {
	table.deck.Shuffle()

	table.addNewPlayers()

	for _, player := range table.activePlayers.ToPlayerArray() {
		player.NewCards()

		player.Action.Amount = 0
		player.Action.Action = playerState.FirstAction // NOTE: set twice w/ new player
	}

	table.newCommunity()

	table.roundCount++

	if table.roundCount%10 == 0 {
		table.Ante *= 2 // TODO increase with time interval instead
		log.Info().Uint64("ante", uint64(table.Ante)).Msg("ante increased")
	}

	table.handleOrphanedSeats()

	table.curPlayers = *table.activePlayers.Clone("curPlayers")
	table.better = nil
	table.Bet = table.Ante // min bet is big blind bet
	table.MainPot.Clear()
	table.MainPot.Bet = table.Bet
	table.sidePots.Clear()
	table.State = TableStateNewRound
}

func (table *Table) FinishRound() {
	table.mtx.Lock()
	defer table.mtx.Unlock()
	// special case for when everyone except a folded player
	// leaves the table
	if table.activePlayers.Len == 1 &&
		table.activePlayers.Head.Player.Action.Action == playerState.Fold {
		log.Warn().
			Str("player", table.activePlayers.Head.Player.Name).
			Msg("only one folded player left at table, abandoning all pots")

		table.State = TableStateGameOver
		table.Winners = []*Player{table.activePlayers.Head.Player}

		return
	}

	players := table.GetNonFoldedPlayers()

	log.Debug().Msgf("mainpot: last bet: %s pot: %s %s",
		printer.Sprintf("%d", table.MainPot.Bet), printer.Sprintf("%d", table.MainPot.Total), table.MainPot.PlayerInfo())
	table.calculateSidePotTotals()
	table.sidePots.Print()
	if table.sidePots.BettingPot != nil &&
		table.sidePots.BettingPot.Total == 0 {
		log.Debug().Msg("removing empty bettingpot")
		table.sidePots.BettingPot = nil
	}

	if len(players) == 1 { // win by folds
		player := players[0]

		player.ChipCount += table.MainPot.Total

		Assert(table.sidePots.IsEmpty(),
			printer.Sprintf("BUG: Table.FinishRound(): %s won by folds but there are sidepots", player.Name))

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
		splitChips := table.MainPot.Total / Chips(len(bestPlayers))

		log.Debug().Msgf("mainpot: split chips: %s", printer.Sprintf("%v", splitChips))

		for _, player := range bestPlayers {
			player.ChipCount += splitChips
		}

		table.State = TableStateSplitPot
	}

	playerMap := make(map[string]*Player)

	table.Winners = bestPlayers
	for _, p := range bestPlayers {
		playerMap[p.Name] = p
	}

	for _, sidePot := range table.sidePots.GetAllPots() {
		// remove players that folded from sidePots
		// XXX: probably not the best place to do this.
		for _, player := range sidePot.Players {
			if player.Action.Action == playerState.Fold {
				log.Debug().
					Str("player", player.Name).
					Str("pot", sidePot.Name).
					Msg("removing folded player from sidepot")
				sidePot.RemovePlayer(player)
			}
		}

		if len(sidePot.Players) == 0 {
			log.Debug().Str("pot", sidePot.Name).Msg("no players attached, skipping")
			continue
		}

		if len(sidePot.Players) == 1 { // win by folds
			var player *Player
			// XXX
			for _, p := range sidePot.Players {
				player = p
			}

			log.Debug().
				Str("player", player.Name).
				Str("pot", sidePot.Name).
				Msg("won by folds")

			player.ChipCount += sidePot.Total

			playerMap[player.Name] = player
		} else {
			bestPlayers := table.BestHand(slices.Collect(maps.Values(sidePot.Players)), sidePot)

			if len(bestPlayers) == 1 {
				log.Debug().
					Str("player", bestPlayers[0].Name).
					Str("pot", sidePot.Name).
					Msg("won sidepot")
				bestPlayers[0].ChipCount += sidePot.Total
			} else {
				splitChips := sidePot.Total / Chips(len(bestPlayers))

				log.Debug().Str("pot", sidePot.Name).Msgf("split chips: %s", printer.Sprintf("%v", splitChips))

				for _, player := range bestPlayers {
					player.ChipCount += splitChips
				}

				//table.State = TableStateSplitPot
			}

			for _, p := range bestPlayers {
				playerMap[p.Name] = p
			}
		}
	}

	table.Winners = slices.Collect(maps.Values(playerMap))
	for _, winner := range table.Winners {
		log.Debug().
			Str("winner", winner.Name).
			Str("chipcount", winner.ChipCountToString()).
			Msg("final chipcount")
	}
}
