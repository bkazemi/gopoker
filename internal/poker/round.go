package poker

import (
	"maps"
	"slices"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/rs/zerolog/log"
)

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
