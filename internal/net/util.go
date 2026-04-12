package net

import (
	"github.com/bkazemi/gopoker/internal/playerState"
)

var netActionToPlayerState map[NetAction]playerState.PlayerState

func init() {
	netActionToPlayerState = map[NetAction]playerState.PlayerState{
		NetDataFirstAction:      playerState.FirstAction,
		NetDataAllIn:            playerState.AllIn,
		NetDataBet:              playerState.Bet,
		NetDataCall:             playerState.Call,
		NetDataCheck:            playerState.Check,
		NetDataFold:             playerState.Fold,
		NetDataVacantSeat:       playerState.VacantSeat,
		NetDataPlayerTurn:       playerState.PlayerTurn,
		NetDataMidroundAddition: playerState.MidroundAddition,
	}
}

func NetActionToPlayerState(netAction NetAction) playerState.PlayerState {
	action, _ := netActionToPlayerState[netAction]

	return action
}
