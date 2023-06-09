package playerState

type PlayerState uint64
const (
  FirstAction PlayerState = 1 << iota
  AllIn
  Bet
  Call
  Check
  Fold
  VacantSeat
  PlayerTurn
  MidroundAddition
)
/*NetDataAllIn:
274:  case net.NetDataBet:
276:  case net.NetDataCall:
278:  case net.NetDataCheck:
280:  case net.NetDataFold:
283:  case net.NetDataVacantSeat:
285:  case net.NetDataPlayerTurn:
287:  case net.NetDataFirstAction:
289:  case net.NetDataMidroundAddition:
)*/
