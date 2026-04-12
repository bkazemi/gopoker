package poker

import (
	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/rs/zerolog/log"
)

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
