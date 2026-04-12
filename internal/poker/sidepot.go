package poker

import (
	"fmt"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/rs/zerolog/log"
)

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

func (table *Table) handleSidePots(player *Player, prevBet Chips, betDiff Chips) {
	if table.sidePots.IsEmpty() { // first sidePot
		sidePot := NewSidePot(table.Bet - player.Action.Amount).WithName("bettingPot")

		if prevBet > 0 {
			log.Debug().
				Str("player", player.Name).
				Uint64("prevBet", uint64(prevBet)).
				Msg("firstSidePot: removing prevBet from mainPot")
			table.MainPot.Total -= prevBet
		}

		if sidePot.Bet == 0 { // first allin was a raise/exact match bet
			log.Debug().
				Str("player", player.Name).
				Msg("firstSidePot: allin created an empty betting sidepot")
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

				log.Debug().
					Str("player", p.Name).
					Msgf("firstSidePot: sub %s from mainpot, add same amt to BettingPot", printer.Sprintf("%d", sidePot.Bet))

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
				log.Debug().
					Str("player", player.Name).
					Uint64("mainPotBet", uint64(mainPot.Bet)).
					Msg("moving previous mainpot to first sidepot")
				betDiff := mainPot.Bet - player.Action.Amount
				log.Debug().
					Str("player", player.Name).
					Uint64("fromBet", uint64(mainPot.Bet)).
					Uint64("toBet", uint64(player.Action.Amount)).
					Uint64("betDiff", uint64(betDiff)).
					Msg("changed mainpot bet")

				sidePot := NewSidePot(mainPot.Bet).WithPlayers(mainPot.Players)

				mainPot.Bet = player.Action.Amount
				oldTotal := mainPot.Total
				mainPot.Total -= betDiff * Chips(len(mainPot.Players))
				mainPot.Total += mainPot.Bet - prevBet // add this player's new chips
				log.Debug().
					Uint64("from", uint64(oldTotal)).
					Uint64("to", uint64(mainPot.Total)).
					Msg("mainpot total changed")
				mainPot.AddPlayer(player)

				table.sidePots.AllInPots.Insert(sidePot, 0)
			} else if mainPot.Bet == player.Action.Amount {
				log.Debug().
					Str("player", player.Name).
					Uint64("allin", uint64(player.Action.Amount)).
					Uint64("prevBet", uint64(prevBet)).
					Msg("allin matched mainpot allin")

				mainPot.AddPlayer(player)
				oldTotal := mainPot.Total
				mainPot.Total += mainPot.Bet - prevBet
				log.Debug().
					Uint64("from", uint64(oldTotal)).
					Uint64("to", uint64(mainPot.Total)).
					Msg("mainpot total changed")
			} else {
				if !mainPot.HasPlayer(player) {
					if prevBet > 0 {
						log.Debug().
							Str("player", player.Name).
							Uint64("prevBet", uint64(prevBet)).
							Msg("allin: adding (mainPot.Bet - prevBet) to mainPot")
					}
					mainPot.AddPlayer(player)
					oldTotal := mainPot.Total
					mainPot.Total += mainPot.Bet - prevBet
					log.Debug().
						Uint64("from", uint64(oldTotal)).
						Uint64("to", uint64(mainPot.Total)).
						Msg("mainpot total changed")
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

					log.Debug().
						Str("player", player.Name).
						Uint64("allin", uint64(player.Action.Amount)).
						Msg("largest AllIn sidePot")

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
							log.Debug().
								Uint64("bet", uint64(sidePot.MustCall.Bet)).
								Int("numPlayers", len(sidePot.MustCall.Players)).
								Msg("created new MustCall pot")
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
						log.Debug().
							Uint64("fromBet", uint64(bettingPot.Bet)).
							Uint64("toBet", uint64(bettingPot.Bet-sidePotBetDiff)).
							Msg("bettingpot bet changed")
						log.Debug().
							Uint64("fromPot", uint64(bettingPot.Total)).
							Uint64("toPot", uint64(bettingPot.Total-(sidePotBetDiff*Chips(len(bettingPot.Players))))).
							Msg("bettingpot pot changed")
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

					log.Debug().
						Str("player", player.Name).
						Uint64("allin", uint64(sidePot.Bet)).
						Int("idx", idx).
						Str("playerInfo", sidePot.PlayerInfo()).
						Msg("inserting allin sidepot")

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
				log.Debug().
					Str("player", player.Name).
					Msg("mpclosed: all in matched a previous all in sidePot")
			case -1:
				var sidePot *SidePot

				log.Debug().
					Str("player", player.Name).
					Str("allin", player.ChipCountToString()).
					Msg("mpclosed: largest AllIn sidePot")

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
						log.Debug().
							Uint64("bet", uint64(sidePot.MustCall.Bet)).
							Int("numPlayers", len(sidePot.MustCall.Players)).
							Msg("mpclosed: created new MustCall pot")
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
					log.Debug().
						Uint64("fromBet", uint64(bettingPot.Bet)).
						Uint64("toBet", uint64(bettingPot.Bet-sidePotBetDiff)).
						Msg("mpclosed: bettingpot bet changed")
					log.Debug().
						Uint64("fromPot", uint64(bettingPot.Total)).
						Uint64("toPot", uint64(bettingPot.Total-(sidePotBetDiff*Chips(len(bettingPot.Players))))).
						Msg("mpclosed: bettingpot pot changed")
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

				log.Debug().
					Str("player", player.Name).
					Uint64("allin", uint64(sidePot.Bet)).
					Int("idx", idx).
					Str("playerInfo", sidePot.PlayerInfo()).
					Msg("mpclosed: inserting allin sidepot")

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
				log.Debug().
					Str("player", player.Name).
					Uint64("mainPotBet", uint64(mainPot.Bet)).
					Uint64("prevBet", uint64(prevBet)).
					Msg("called allin reraise, adding to mainPot")
				oldTotal := mainPot.Total
				mainPot.Total += (table.MainPot.Bet - prevBet)
				log.Debug().
					Uint64("from", uint64(oldTotal)).
					Uint64("to", uint64(mainPot.Total)).
					Msg("mainpot total changed")
			} else { // player hadn't added to the previous (smaller) mainpot bet
				oldTotal := mainPot.Total
				mainPot.Total += mainPot.Bet
				log.Debug().
					Uint64("from", uint64(oldTotal)).
					Uint64("to", uint64(mainPot.Total)).
					Msg("mainpot total changed")
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
				log.Debug().
					Str("player", player.Name).
					Uint64("allinBet", uint64(sidePot.Bet)).
					Msg("removing from allin MustCall struct")
				sidePot.MustCall.RemovePlayer(player)
			}

			if !sidePot.HasPlayer(player) {
				sidePot.AddPlayer(player)
			} else {
				log.Debug().
					Str("player", player.Name).
					Uint64("bet", uint64(sidePot.Bet)).
					Msg("player already in sidePot")
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
					log.Debug().
						Uint64("sidePotDiff", uint64(sidePotDiff)).
						Msg("bettingpot: sidePotDiff == 0 && !mpclosed")
					sidePotDiff = table.MainPot.Bet
				} else {
					log.Debug().
						Uint64("sidePotDiff", uint64(sidePotDiff)).
						Msg("bettingpot: sidePotDiff != 0 || mpclosed")
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
				log.Debug().
					Str("player", player.Name).
					Msgf("bettingpot: changed bet from %s to %s", printer.Sprintf("%d", lastBettingPotBet), printer.Sprintf("%d", bettingPot.Bet))
			}
		case playerState.Call:
			if bettingPot.Bet == 0 {
				log.Debug().
					Str("player", player.Name).
					Msg("bettingpot: called, but pot is empty. no bettingpot change")
			} else {
				// XXX there was a problem here, but I've forgetten what it was.
				log.Debug().
					Str("player", player.Name).
					Uint64("chips", uint64(bettingPot.Bet)).
					Msg("bettingpot: called, adding chips")
				bettingPot.Total += bettingPot.Bet
				//bettingPot.Total += betDiff
			}
		}
	}
}
