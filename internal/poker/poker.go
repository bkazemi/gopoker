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

	"github.com/rivo/uniseg"
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

func (table *Table) closeSidePots() {
	if table.sidePots.BettingPot == nil { // no sidepots yet
		return
	}

	if !table.MainPot.IsClosed {
		log.Debug().Msg("closing mainpot")
		table.MainPot.IsClosed = true
	}

	// XXX move me
	//if table.allInCount() == 1 {
	// all players called the all-in player
	//
	// }

	table.sidePots.AllInPots.CloseAll()

	table.sidePots.BettingPot.Bet = 0
}

// NOTE: calculated at end of each community betting stages
func (table *Table) calculateSidePotTotals() {
	if table.sidePots.IsEmpty() {
		return
	}

	openSidePots := table.sidePots.AllInPots.GetOpenPots()
	if len(openSidePots) == 0 {
		return
	}

	var prevBet Chips
	if !table.MainPot.IsClosed {
		prevBet = table.MainPot.Bet
	} else {
		firstSidePot := openSidePots[0]

		firstSidePot.Calculate(0)

		if len(openSidePots) == 1 {
			return
		}

		prevBet = firstSidePot.Bet
		openSidePots = openSidePots[1:]
	}

	for _, sidePot := range openSidePots {
		sidePot.Calculate(prevBet)
		prevBet = sidePot.Bet
	}
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
		log.Debug().Str("player", p.Name).Uint64("actionAmt", uint64(p.Action.Amount)).Msg("getChipLeaders")
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
		log.Debug().Uint("lenRet", uint(len(ret))).Uint8("np-1", table.NumPlayers-1).Msg("GetEliminatedPlayers")
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

			log.Debug().Str("smallBlind", table.SmallBlind.Player.Name).Str("curPlayer", smallBlindNode.Player.Name).Msg("smallBlind not active")
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

	log.Debug().Str("dealer", table.Dealer.Player.Name).Str("smallBlind", table.SmallBlind.Player.Name).Str("bigBlind", table.BigBlind.Player.Name).Msg("before")

	Panic := &Panic{}

	defer Panic.IfNoPanic(func() {
		log.Debug().Str("dealer", table.Dealer.Player.Name).Str("smallBlind", table.SmallBlind.Player.Name).Str("bigBlind", table.BigBlind.Player.Name).Msg("after")

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
				log.Debug().Str("removed", thisPlayer.Player.Name).Str("headWas", table.curPlayers.Head.Player.Name).Msg("allIn removal")
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
			log.Debug().Str("removed", thisPlayer.Player.Name).Str("headWas", table.curPlayers.Head.Player.Name).Msg("fold removal")
			table.curPlayer = nextNode
			log.Debug().Str("headNow", table.curPlayers.Head.Player.Name).Msg("curPlayers head updated after fold")
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
		log.Debug().Str("curPlayersHead", table.curPlayers.Head.Player.Name).Str("curPlayerNext", table.curPlayer.Next().Player.Name).Msg("done betting")

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
				log.Debug().Str("player", player.Name).Str("from", cc).Str("to", player.ChipCountToString()).Msg("chipcount changed")
			}
		}()
	}

	var blindRequiredBet Chips = 0

	isSmallBlindPreFlop := false

	if table.CommState == TableStatePreFlop &&
		table.State != TableStatePlayerRaised { // XXX mixed states...
		if table.SmallBlind != nil && player.Name == table.SmallBlind.Player.Name {
			isSmallBlindPreFlop = true
			blindRequiredBet = MinChips(table.SmallBlind.Player.ChipCount, table.Ante/2)
		} else if table.BigBlind != nil && player.Name == table.BigBlind.Player.Name {
			blindRequiredBet = MinChips(table.BigBlind.Player.ChipCount, table.Ante)
		}
	}

	handleSidePots := func(prevBet Chips, betDiff Chips) {
		if table.sidePots.IsEmpty() { // first sidePot
			sidePot := NewSidePot(table.Bet - player.Action.Amount).WithName("bettingPot")

			if prevBet > 0 {
				log.Debug().Str("player", player.Name).Uint64("prevBet", uint64(prevBet)).Msg("firstSidePot: removing prevBet from mainPot")
				table.MainPot.Total -= prevBet
			}

			if sidePot.Bet == 0 { // first allin was a raise/exact match bet
				log.Debug().Str("player", player.Name).Msg("firstSidePot: allin created an empty betting sidepot")
			} else {
				// get players who already called the last bet,
				// sub the delta of the last bet and this players
				// chipcount in mainpot, then add them to the mainpot & sidepot.
				for playerNode := table.curPlayers.Head; playerNode.Player.Name != player.Name; playerNode = playerNode.Next() {
					p := playerNode.Player
					if p.Name == player.Name {
						break
					}

					table.MainPot.Total -= sidePot.Bet

					log.Debug().Str("player", p.Name).Msgf("firstSidePot: sub %s from mainpot, add same amt to BettingPot", printer.Sprintf("%d", sidePot.Bet))

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

		mainPot, bettingPot := table.MainPot, table.sidePots.BettingPot

		if player.Action.Action == playerState.AllIn {
			if !mainPot.IsClosed {
				if mainPot.Bet > player.Action.Amount {
					log.Debug().Str("player", player.Name).Uint64("mainPotBet", uint64(mainPot.Bet)).Msg("moving previous mainpot to first sidepot")
					betDiff := mainPot.Bet - player.Action.Amount
					log.Debug().Str("player", player.Name).Uint64("fromBet", uint64(mainPot.Bet)).Uint64("toBet", uint64(player.Action.Amount)).Uint64("betDiff", uint64(betDiff)).Msg("changed mainpot bet")

					sidePot := NewSidePot(mainPot.Bet).WithPlayers(mainPot.Players)

					mainPot.Bet = player.Action.Amount
					oldTotal := mainPot.Total
					mainPot.Total -= betDiff * Chips(len(mainPot.Players))
					mainPot.Total += mainPot.Bet - prevBet // add this player's new chips
					log.Debug().Uint64("from", uint64(oldTotal)).Uint64("to", uint64(mainPot.Total)).Msg("mainpot total changed")
					mainPot.AddPlayer(player)

					table.sidePots.AllInPots.Insert(sidePot, 0)
				} else if mainPot.Bet == player.Action.Amount {
					log.Debug().Str("player", player.Name).Uint64("allin", uint64(player.Action.Amount)).Uint64("prevBet", uint64(prevBet)).Msg("allin matched mainpot allin")

					mainPot.AddPlayer(player)
					oldTotal := mainPot.Total
					mainPot.Total += mainPot.Bet - prevBet
					log.Debug().Uint64("from", uint64(oldTotal)).Uint64("to", uint64(mainPot.Total)).Msg("mainpot total changed")
				} else {
					if !mainPot.HasPlayer(player) {
						if prevBet > 0 {
							log.Debug().Str("player", player.Name).Uint64("prevBet", uint64(prevBet)).Msg("allin: adding (mainPot.Bet - prevBet) to mainPot")
						}
						mainPot.AddPlayer(player)
						oldTotal := mainPot.Total
						mainPot.Total += mainPot.Bet - prevBet
						log.Debug().Uint64("from", uint64(oldTotal)).Uint64("to", uint64(mainPot.Total)).Msg("mainpot total changed")
					}

					idx := -1
					for i, sidePot := range table.sidePots.AllInPots.Pots {
						if sidePot.IsClosed {
							continue
						}

						if sidePot.Bet <= player.Action.Amount {
							sidePot.AddPlayer(player)
							if sidePot.Bet == player.Action.Amount {
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
						log.Debug().Str("player", player.Name).Msg("all in matched a previous all-in sidePot")
					case -1:
						var sidePot *SidePot

						log.Debug().Str("player", player.Name).Uint64("allin", uint64(player.Action.Amount)).Msg("largest AllIn sidePot")

						var (
							sidePotBetDiff Chips = player.Action.Amount - table.MainPot.Bet
						)

						if largestSidePot := table.sidePots.AllInPots.GetLargest(); largestSidePot != nil {
							sidePotBetDiff = player.Action.Amount - largestSidePot.Bet
						}

						if sidePotBetDiff == bettingPot.Bet {
							log.Debug().Msg("allin == bettingpot bet")
							sidePot = NewSidePot(player.Action.Amount).
								WithPlayers(bettingPot.Players).
								WithPlayer(player)
						} else if sidePotBetDiff > bettingPot.Bet {
							log.Debug().Msg("allin > bettingpot bet")
							sidePot = NewSidePot(player.Action.Amount).WithPlayer(player)
							if bettingPot.Bet != 0 {
								sidePot.MustCall = NewPot(fmt.Sprintf("%s mustcall pot", sidePot.Name), bettingPot.Bet).
									WithPlayers(bettingPot.Players)
								log.Debug().Uint64("bet", uint64(sidePot.MustCall.Bet)).Int("numPlayers", len(sidePot.MustCall.Players)).Msg("created new MustCall pot")
							}
							bettingPot.Clear()
							sidePotBetDiff = 0
						} else {
							log.Debug().Msg("allin < bettingpot bet")
							sidePot = NewSidePot(player.Action.Amount).
								WithPlayers(bettingPot.Players).
								WithPlayer(player)
						}

						if sidePotBetDiff != 0 {
							// bettingPot bet is always delta largest sidePot
							// that is also less than bettingPot bet
							log.Debug().Uint64("fromBet", uint64(bettingPot.Bet)).Uint64("toBet", uint64(bettingPot.Bet-sidePotBetDiff)).Msg("bettingpot bet changed")
							log.Debug().Uint64("fromPot", uint64(bettingPot.Total)).Uint64("toPot", uint64(bettingPot.Total-(sidePotBetDiff*Chips(len(bettingPot.Players))))).Msg("bettingpot pot changed")
							bettingPot.Bet -= sidePotBetDiff
							bettingPot.Total -= sidePotBetDiff * Chips(len(bettingPot.Players))
						}

						table.sidePots.AllInPots.Add(sidePot)
					default:
						sidePot := NewSidePot(player.Action.Amount).
							WithPlayers(bettingPot.Players).
							WithPlayer(player)

						/* allin players get automatically added to the smaller allin sidePots
						   for _, sidePot := range table.sidePots.AllInPots.Pots[:idx] {
						     printer.Printf("  <%s> adding to %v allin sidepot\n", player.Name, sidePot.Bet)
						     sidePot.AddPlayer(player)
						   }*/

						// that goes for this sidePot as well
						// (including the bettingpot which is included in the factory function)
						for _, sp := range table.sidePots.AllInPots.GetPotsStartingAt(idx) {
							sidePot.AddPlayers(sp.Players)
						}

						log.Debug().Str("player", player.Name).Uint64("allin", uint64(sidePot.Bet)).Int("idx", idx).Str("playerInfo", sidePot.PlayerInfo()).Msg("inserting allin sidepot")

						table.sidePots.AllInPots.Insert(sidePot, idx)
					}
				}
			} else { // mainpot closed
				idx := -1
				for i, sidePot := range table.sidePots.AllInPots.Pots {
					if sidePot.IsClosed {
						continue
					}

					if sidePot.Bet <= player.Action.Amount {
						// if a sidePot has a MustCall struct, then the sidePot raise
						// will include the MustCall bet so this player does not need
						// to be in the MustCall struct anymore
						if sidePot.MustCall != nil && sidePot.MustCall.HasPlayer(player) {
							sidePot.MustCall.RemovePlayer(player)
						}
						sidePot.AddPlayer(player)
						if sidePot.Bet == player.Action.Amount {
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
					log.Debug().Str("player", player.Name).Msg("mpclosed: all in matched a previous all in sidePot")
				case -1:
					var sidePot *SidePot

					log.Debug().Str("player", player.Name).Str("allin", player.ChipCountToString()).Msg("mpclosed: largest AllIn sidePot")

					var (
						bettingPotBetDiff Chips = player.Action.Amount
						sidePotBetDiff    Chips = player.Action.Amount
					)

					if largestSidePot := table.sidePots.AllInPots.GetLargest(); largestSidePot != nil {
						sidePotBetDiff -= largestSidePot.Bet
					}

					if sidePotBetDiff == bettingPot.Bet {
						log.Debug().Msg("mpclosed: allin == bettingpot bet")
						sidePot = NewSidePot(player.Action.Amount).
							WithPlayers(bettingPot.Players).
							WithPlayer(player)
					} else if sidePotBetDiff > bettingPot.Bet {
						log.Debug().Msg("mpclosed: allin > bettingpot bet")
						sidePot = NewSidePot(player.Action.Amount).WithPlayer(player)
						if bettingPot.Bet != 0 {
							sidePot.MustCall = NewPot(fmt.Sprintf("%s mustcall pot", sidePot.Name), bettingPot.Bet).
								WithPlayers(bettingPot.Players)
							log.Debug().Uint64("bet", uint64(sidePot.MustCall.Bet)).Int("numPlayers", len(sidePot.MustCall.Players)).Msg("mpclosed: created new MustCall pot")
						}
						bettingPot.Clear()
						bettingPotBetDiff = 0
					} else {
						log.Debug().Msg("mpclosed: allin < bettingpot bet")
						sidePot = NewSidePot(player.Action.Amount).
							WithPlayers(bettingPot.Players).
							WithPlayer(player)
					}

					if bettingPotBetDiff != 0 {
						// bettingPot bet is always delta largest sidePot
						// that is also less than bettingPot bet
						log.Debug().Uint64("fromBet", uint64(bettingPot.Bet)).Uint64("toBet", uint64(bettingPot.Bet-sidePotBetDiff)).Msg("mpclosed: bettingpot bet changed")
						log.Debug().Uint64("fromPot", uint64(bettingPot.Total)).Uint64("toPot", uint64(bettingPot.Total-(sidePotBetDiff*Chips(len(bettingPot.Players))))).Msg("mpclosed: bettingpot pot changed")
						bettingPot.Bet -= sidePotBetDiff
						bettingPot.Total -= sidePotBetDiff * Chips(len(bettingPot.Players))
					}

					table.sidePots.AllInPots.Add(sidePot)
				default:
					sidePot := NewSidePot(player.Action.Amount).
						WithPlayers(bettingPot.Players).
						WithPlayer(player)

					// everyone in larger allins are automatically added to smaller sidePots
					// (including the bettingpot which is included in the factory function)
					for _, sp := range table.sidePots.AllInPots.GetPotsStartingAt(idx) {
						sidePot.AddPlayers(sp.Players)
					}

					log.Debug().Str("player", player.Name).Uint64("allin", uint64(sidePot.Bet)).Int("idx", idx).Str("playerInfo", sidePot.PlayerInfo()).Msg("mpclosed: inserting allin sidepot")

					table.sidePots.AllInPots.Insert(sidePot, idx)
				}
			}
		} else { // not an allin
			if !mainPot.IsClosed && !mainPot.HasPlayer(player) {
				Assert(player.ChipCount >= mainPot.Bet,
					printer.Sprintf("Table.PlayerAction(): handleSidePots(): <%v> cc: %v cant match mainpot bet %v",
						player.Name, player.ChipCount, mainPot.Bet))
				//Assert(mainPot.Bet > betDiff,
				//       printer.Sprintf("Table.PlayerAction(): handleSidePots(): <%s> betDiff %v > mainPot bet: %v",
				//                      player.Name, betDiff, mainPot.Bet))

				if player.Action.Action != playerState.FirstAction && table.MainPot.Bet > prevBet {
					log.Debug().Str("player", player.Name).Uint64("mainPotBet", uint64(mainPot.Bet)).Uint64("prevBet", uint64(prevBet)).Msg("called allin reraise, adding to mainPot")
					oldTotal := mainPot.Total
					mainPot.Total += (table.MainPot.Bet - prevBet)
					log.Debug().Uint64("from", uint64(oldTotal)).Uint64("to", uint64(mainPot.Total)).Msg("mainpot total changed")
				} else { // player hadn't added to the previous (smaller) mainpot bet
					oldTotal := mainPot.Total
					mainPot.Total += mainPot.Bet
					log.Debug().Uint64("from", uint64(oldTotal)).Uint64("to", uint64(mainPot.Total)).Msg("mainpot total changed")
				}
				mainPot.AddPlayer(player)
			}

			if !bettingPot.HasPlayer(player) {
				bettingPot.AddPlayer(player)
			}

			// add current player to open sidepots. this happens when multiple
			// players go all-in.
			for _, sidePot := range table.sidePots.AllInPots.GetOpenPots() {
				Assert(player.ChipCount >= sidePot.Bet, "player cant match a sidePot bet")

				// if a sidePot has a MustCall struct, then the sidePot raise
				// will include the MustCall bet so this player does not need
				// to be included anymore
				if sidePot.MustCall != nil && sidePot.MustCall.HasPlayer(player) {
					log.Debug().Str("player", player.Name).Uint64("allinBet", uint64(sidePot.Bet)).Msg("removing from allin MustCall struct")
					sidePot.MustCall.RemovePlayer(player)
				}

				if !sidePot.HasPlayer(player) {
					sidePot.AddPlayer(player)
				} else {
					log.Debug().Str("player", player.Name).Uint64("bet", uint64(sidePot.Bet)).Msg("player already in sidePot")
				}
			}

			switch player.Action.Action {
			case playerState.Bet:
				lastBettingPotBet := bettingPot.Bet
				if table.State == TableStatePlayerRaised &&
					player.Action.Amount > bettingPot.Bet {
					log.Debug().Str("player", player.Name).Msg("bettingpot: re-raised")
					sidePotDiff := Chips(0)
					for _, sidePot := range table.sidePots.AllInPots.GetOpenPots() {
						// NOTE: sidePots are ordered so we only need this comparison
						if sidePot.Bet < bettingPot.Bet {
							sidePotDiff = sidePot.Bet
						}
					}
					if sidePotDiff == 0 && !table.MainPot.IsClosed {
						log.Debug().Uint64("sidePotDiff", uint64(sidePotDiff)).Msg("bettingpot: sidePotDiff == 0 && !mpclosed")
						sidePotDiff = table.MainPot.Bet
					} else {
						log.Debug().Uint64("sidePotDiff", uint64(sidePotDiff)).Msg("bettingpot: sidePotDiff != 0 || mpclosed")
					}
					if bettingPot.Bet == 0 {
						log.Debug().Str("player", player.Name).Msg("bettingpot: re-raised an all-in")
					}
					Assert(sidePotDiff <= player.Action.Amount,
						printer.Sprintf("Table.PlayerAction(): handleSidePots(): bettingpot: sidePotDiff %v is greater than %s action.amt (%v)\n",
							sidePotDiff, player.Name, player.Action.Amount))
					bettingPot.Total += player.Action.Amount - sidePotDiff
					bettingPot.Bet = player.Action.Amount - sidePotDiff
				} else {
					log.Debug().Str("player", player.Name).Msg("bettingpot: made new bet")
					bettingPot.Bet = player.Action.Amount
					bettingPot.Total += bettingPot.Bet
				}
				if bettingPot.Bet != lastBettingPotBet {
					log.Debug().Str("player", player.Name).Msgf("bettingpot: changed bet from %s to %s", printer.Sprintf("%d", lastBettingPotBet), printer.Sprintf("%d", bettingPot.Bet))
				}
			case playerState.Call:
				if bettingPot.Bet == 0 {
					log.Debug().Str("player", player.Name).Msg("bettingpot: called, but pot is empty. no bettingpot change")
				} else {
					// XXX there was a problem here, but I've forgetten what it was.
					log.Debug().Str("player", player.Name).Uint64("chips", uint64(bettingPot.Bet)).Msg("bettingpot: called, adding chips")
					bettingPot.Total += bettingPot.Bet
					//bettingPot.Total += betDiff
				}
			}
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
			player.Action.Amount = MinChips(table.Bet, player.ChipCount)
		} else {
			chipLeaderCount, secondChipLeaderCount := table.getChipLeaders(true)
			log.Debug().Str("player", player.Name).Uint64("chipLeader", uint64(chipLeaderCount)).Uint64("2ndChipLeader", uint64(secondChipLeaderCount)).Msg("allin chip leaders")

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
					log.Debug().Str("player", player.Name).Str("newHead", table.curPlayer.Player.Name).Msg("allin: setting curPlayers head")
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

		handleSidePots(prevChips, 0)

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
		log.Debug().Str("player", player.Name).Uint64("chipLeader", uint64(chipLeaderCount)).Uint64("2ndChipLeader", uint64(secondChipLeaderCount)).Msg("bet chip leaders")

		log.Debug().Str("player", player.Name).Uint64("prevChips", uint64(prevChips)).Msg("bet: adding prevChips")
		player.ChipCount += prevChips

		// NOTE: A chipleader can only bet what at least one other player can match.
		if player.ChipCount == chipLeaderCount {
			player.Action.Amount = MinChips(action.Amount, secondChipLeaderCount)
		} else {
			player.Action.Amount = action.Amount
		}

		if action.Amount == player.ChipCount {
			player.Action.Action = playerState.AllIn
		} else {
			player.Action.Action = playerState.Bet
		}

		if player.Action.Action == playerState.AllIn || !table.sidePots.IsEmpty() {
			handleSidePots(prevChips, 0)
		} else {
			table.MainPot.Total += player.Action.Amount - prevChips
		}

		player.ChipCount -= player.Action.Amount

		table.Bet = player.Action.Amount

		log.Debug().Str("player", player.Name).Str("newHead", table.curPlayer.Player.Name).Msg("bet: setting curPlayers head")
		table.curPlayers.SetHead(table.curPlayer)
		table.better = player
		table.State = TableStatePlayerRaised
	case playerState.Call:
		if table.State != TableStatePlayerRaised && !isSmallBlindPreFlop {
			return errors.New("nothing to call")
		}

		if (table.SmallBlind != nil && player.Name == table.SmallBlind.Player.Name) ||
			(table.BigBlind != nil && player.Name == table.BigBlind.Player.Name) {
			log.Debug().Str("player", player.Name).Uint64("actionAmt", uint64(player.Action.Amount)).Msg("call: blind")
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

			handleSidePots(prevChips, 0)

			player.ChipCount = 0
		} else {
			player.Action.Action = playerState.Call
			player.Action.Amount = table.Bet

			if !table.sidePots.IsEmpty() {
				handleSidePots(prevChips, betDiff)
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

		table.SmallBlind.Player.Action.Amount = MinChips(table.Ante/2,
			table.SmallBlind.Player.ChipCount)
		table.BigBlind.Player.Action.Amount = MinChips(table.Ante,
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

		table.SmallBlind.Player.Action.Amount = MinChips(table.Ante/2,
			table.SmallBlind.Player.ChipCount)
		table.BigBlind.Player.Action.Amount = MinChips(table.Ante,
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
		log.Warn().Str("player", table.activePlayers.Head.Player.Name).Msg("only one folded player left at table, abandoning all pots")

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
				log.Debug().Str("player", player.Name).Str("pot", sidePot.Name).Msg("removing folded player from sidepot")
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

			log.Debug().Str("player", player.Name).Str("pot", sidePot.Name).Msg("won by folds")

			player.ChipCount += sidePot.Total

			playerMap[player.Name] = player
		} else {
			sidePotPlayersArr := make([]*Player, 0, len(sidePot.Players))
			for _, player := range sidePot.Players { // TODO: make a mapval2slice util func
				sidePotPlayersArr = append(sidePotPlayersArr, player)
			}
			bestPlayers := table.BestHand(sidePotPlayersArr, sidePot)

			if len(bestPlayers) == 1 {
				log.Debug().Str("player", bestPlayers[0].Name).Str("pot", sidePot.Name).Msg("won sidepot")
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

	table.Winners = PlayerMapToArr(playerMap)
	for _, winner := range table.Winners {
		log.Debug().Str("winner", winner.Name).Str("chipcount", winner.ChipCountToString()).Msg("final chipcount")
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

	// XXX move me
	maxNameWidth := 0
	for _, player := range players {
		maxNameWidth = MaxInt(uniseg.StringWidth(player.Name), maxNameWidth)
	}

	if sidePot == nil {
		for _, player := range players {
			AssembleBestHand(false, table, player)

			nameField := FillRight(player.Name, maxNameWidth)

			*winInfo += fmt.Sprintf("%s [%4s][%4s] => %-15s (rank %d)\n",
				nameField,
				player.Hole.Cards[0].Name, player.Hole.Cards[1].Name,
				player.Hand.RankName(), player.Hand.Rank)

			log.Debug().Str("player", nameField).
				Str("hole", fmt.Sprintf("[%4s][%4s]", player.Hole.Cards[0].Name, player.Hole.Cards[1].Name)).
				Str("hand", player.Hand.RankName()).Int("rank", int(player.Hand.Rank)).
				Msg("player hand")
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
		names := ""
		*winInfo += "split pot between "
		for _, player := range tiedPlayers {
			names += player.Name + " "
			*winInfo += player.Name + " "
		}
		log.Info().Str("players", names).Str("hand", tiedPlayers[0].Hand.RankName()).Msg("split pot")

		*winInfo += "\nwinning hand => " + tiedPlayers[0].Hand.RankName() + "\n"
	} else {
		*winInfo += "\n" + tiedPlayers[0].Name + "  wins with " + tiedPlayers[0].Hand.RankName() + "\n"
		log.Info().Str("player", tiedPlayers[0].Name).Str("hand", tiedPlayers[0].Hand.RankName()).Msg("winner")
	}

	// build best hand string
	handStr := ""
	for _, card := range reverseCards(tiedPlayers[0].Hand.Cards) {
		handStr += fmt.Sprintf("[%4s]", card.Name)
		*winInfo += fmt.Sprintf("[%4s]", card.Name)
	}
	log.Debug().Str("cards", handStr).Msg("best hand")

	return tiedPlayers
}

// hand matching logic unoptimized
func AssembleBestHand(preShow bool, table *Table, player *Player) {
	if preShow {
		cloneHand := func(hand *Hand) Hand {
			if hand == nil {
				return Hand{}
			}

			cloned := *hand
			cloned.Cards = append(Cards(nil), hand.Cards...)

			return cloned
		}

		var restoreHand Hand
		if player.Hand != nil {
			restoreHand = cloneHand(player.Hand)
		} else {
			restoreHand = Hand{}
		}

		defer func() {
			if preShow {
				if table.State == TableStatePreFlop && len(player.Hole.Cards) == 2 {
					player.preHand = &Hand{}
					if player.Hole.Cards[0].NumValue == player.Hole.Cards[1].NumValue {
						player.preHand.Rank = RankPair
					}
				} else if player.Hand != nil {
					previewHand := cloneHand(player.Hand)
					player.preHand = &previewHand
				} else {
					player.preHand = &Hand{}
				}
				player.Hand = &restoreHand
			}
		}()
	}

	if table.State == TableStatePreFlop ||
		len(player.Hole.Cards) != 2 ||
		len(table.Community) < 3 {
		return
	}

	cards := append(Cards{}, table.Community...)
	cards = append(cards, player.Hole.Cards...)
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
	top_cards := func(cards Cards, num int, except []CardVal) Cards {
		ret := make(Cards, 0, 5)

		Assert(len(cards) <= 7, "too many cards in top_cards()")

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
	gotFlush := func(cards Cards, player *Player, addToCards bool) (bool, Suit) {
		type _suitstruct struct {
			cnt   uint
			cards Cards
		}
		suits := make(map[Suit]*_suitstruct)

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
		Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

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

		// check for A to 5
		if !isStraight && cards[len(cards)-1].NumValue == CardAce {
			gotStraight(&cards, player, 3, true)
		}

		if player.Hand.Rank == RankRoyalFlush ||
			player.Hand.Rank == RankStraightFlush {
			return
		}

		if isFlush, _ := gotFlush(cards, player, false); isFlush {
			// replace any previously assembled straight cards before storing flush cards.
			player.Hand.Cards = player.Hand.Cards[:0]
			gotFlush(cards, player, true)
			return
		}

		if player.Hand.Rank == RankStraight {
			return
		}

		// muck
		player.Hand.Rank = RankHighCard
		player.Hand.Cards = append(player.Hand.Cards, cards[bestCard-5],
			cards[bestCard-4], cards[bestCard-3],
			cards[bestCard-2], cards[bestCard-1])

		Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

		return
	}

	// quads search //
	if matchHands.quads != nil {
		quadsIdx := int(matchHands.quads[0]) // 0 because it's impossible to
		// get quads twice
		kicker := &Card{}
		for i := bestCard - 1; i >= 0; i-- { // kicker search
			if cards[i].NumValue != cards[quadsIdx].NumValue {
				kicker = cards[i]
				break
			}
		}

		Assert(kicker != nil, "quads: kicker == nil")

		player.Hand.Rank = RankQuads
		player.Hand.Cards = append(Cards{kicker}, cards[quadsIdx:quadsIdx+4]...)

		return
	}

	// fullhouse search //
	//
	// NOTE: we check for a fullhouse before a straight flush because it's
	// impossible to have both at the same time and searching for the fullhouse
	// first saves some cycles+space
	if matchHands.trips != nil && (matchHands.pairs != nil || len(matchHands.trips) > 1) {
		player.Hand.Rank = RankFullHouse

		tripsIdx := int(matchHands.trips[len(matchHands.trips)-1])
		pairIdx := -1

		// Choose the best available pair component for the full house:
		// a second trips can supply the pair, but a higher actual pair should win.
		if len(matchHands.trips) > 1 {
			pairIdx = int(matchHands.trips[len(matchHands.trips)-2])
		}
		if matchHands.pairs != nil {
			highestPairIdx := int(matchHands.pairs[len(matchHands.pairs)-1])
			if pairIdx == -1 || cards[highestPairIdx].NumValue > cards[pairIdx].NumValue {
				pairIdx = highestPairIdx
			}
		}

		Assert(pairIdx != -1, "fullhouse: pairIdx == -1")

		player.Hand.Cards = append(player.Hand.Cards, cards[pairIdx:pairIdx+2]...)
		player.Hand.Cards = append(player.Hand.Cards, cards[tripsIdx:tripsIdx+3]...)

		Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

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
		cardmap := make(map[CardVal]Suit) // key == num, val == suit

		for _, card := range cards {
			mappedsuit, found := cardmap[card.NumValue]

			if found && mappedsuit != suit && card.Suit == suit {
				cardmap[card.NumValue] = card.Suit
				Assert(uniqueCards[len(uniqueCards)-1].NumValue == card.NumValue, "uniqueCards problem")
				uniqueCards[len(uniqueCards)-1] = card // should _always_ be last card
			} else if !found {
				cardmap[card.NumValue] = card.Suit
				uniqueCards = append(uniqueCards, card)
			}
		}

		Assert((len(uniqueCards) <= 7 && len(uniqueCards) >= 3),
			fmt.Sprintf("impossible number of unique cards (%v)", len(uniqueCards)))
	} else {
		cardmap := make(map[CardVal]bool)

		for _, card := range cards {
			if _, val := cardmap[card.NumValue]; !val {
				cardmap[card.NumValue] = true
				uniqueCards = append(uniqueCards, card)
			}
		}

		Assert((len(uniqueCards) <= 7 && len(uniqueCards) >= 1),
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
				Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
				isStraight = true
				break
			}
		}

		if player.Hand.Rank == RankRoyalFlush ||
			player.Hand.Rank == RankStraightFlush {
			return
		}

		if !isStraight && uniqueCards[uniqueBestCard-1].NumValue == CardAce &&
			gotStraight(&uniqueCards, player, 3, true) {
			Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
		}

		if player.Hand.Rank == RankRoyalFlush ||
			player.Hand.Rank == RankStraightFlush {
			return
		}
	}

	if haveFlush {
		// replace any previously assembled straight cards before storing flush cards.
		player.Hand.Cards = player.Hand.Cards[:0]
		gotFlush(cards, player, true)

		Assert(player.Hand.Rank == RankFlush, "player should have a flush")

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

		kickers := top_cards(cards, 2, []CardVal{cards[firstCard].NumValue})
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

			kicker := top_cards(cards, 1, []CardVal{cards[highPairIdx].NumValue,
				cards[lowPairIdx].NumValue})
			player.Hand.Cards = append(kicker, player.Hand.Cards...)
		} else {
			player.Hand.Rank = RankPair
			pairidx := matchHands.pairs[0]
			kickers := top_cards(cards, 3, []CardVal{cards[pairidx].NumValue})
			player.Hand.Cards = append(kickers, cards[pairidx:pairidx+2]...)
		}

		return
	}

	// muck
	player.Hand.Rank = RankHighCard
	player.Hand.Cards = append(player.Hand.Cards, cards[bestCard-5],
		cards[bestCard-4], cards[bestCard-3],
		cards[bestCard-2], cards[bestCard-1])

	return
}
