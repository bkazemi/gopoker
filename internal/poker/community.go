package poker

import (
	"fmt"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/rs/zerolog/log"
)

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
