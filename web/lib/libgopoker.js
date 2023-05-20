import { encode } from '@msgpack/msgpack';

export const NETDATA = {
  CLOSE:           1n << 0n,
  NEWCONN:         1n << 1n,

  YOUR_PLAYER:     1n << 2n,
  NEW_PLAYER:      1n << 3n,
  CUR_PLAYERS:     1n << 4n,
  UPDATE_PLAYER:   1n << 5n,
  UPDATE_TABLE:    1n << 6n,
  PLAYER_LEFT:     1n << 7n,
  CLIENT_EXITED:   1n << 8n,
  CLIENT_SETTINGS: 1n << 9n,
  RESET:           1n << 10n,

  SERVER_CLOSED:   1n << 11n,

  TABLE_LOCKED:    1n << 12n,
  BAD_AUTH:        1n << 13n,
  MAKE_ADMIN:      1n << 14n,
  START_GAME:      1n << 15n,

  CHAT_MSG:        1n << 16n,

  PLAYER_ACTION:   1n << 17n,
  PLAYER_TURN:     1n << 18n,
  PLAYER_HEAD:     1n << 19n,
  ALLIN:           1n << 20n,
  BET:             1n << 21n,
  CALL:            1n << 22n,
  CHECK:           1n << 23n,
  RAISE:           1n << 24n,
  FOLD:            1n << 25n,

  CUR_HAND:           1n << 26n,
  SHOW_HAND:          1n << 27n,

  FIRST_ACTION:       1n << 28n,
  MIDROUND_ADDITION:  1n << 29n,
  ELIMINATED:         1n << 30n,
  VACANT_SEAT:        1n << 31n,

  DEAL:               1n << 32n,
  FLOP:               1n << 33n,
  TURN:               1n << 34n,
  RIVER:              1n << 35n,
  BEST_HAND:          1n << 36n,
  ROUND_OVER:         1n << 37n,

  SERVER_MSG:         1n << 38n,
  BAD_REQUEST:        1n << 39n,

  ROOM_SETTINGS:      1n << 40n,
};

export const NetDataToString = (netDataReqOrRes) => {
  return Object.keys(NETDATA).find(k => NETDATA[k] === netDataReqOrRes);
};

export const PlayerActionToString = (action) => {
  switch (action.Action) {
  case NETDATA.ALLIN:
    return `all in (${action.Amount.toLocaleString()} chips)`;
  case NETDATA.BET:
    return `raise (bet ${action.Amount.toLocaleString()} chips)`;
  case NETDATA.CALL:
    return `call (${action.Amount.toLocaleString()} chips)`;
  case NETDATA.CHECK:
    return 'check';
  case NETDATA.FOLD:
    return 'fold';

  case NETDATA.VACANT_SEAT:
    return 'N/A';
  case NETDATA.PLAYER_TURN:
    return '(player\'s turn) waiting for action';
  case NETDATA.FIRST_ACTION:
    return 'waiting for first action';
  case NETDATA.MIDROUND_ADDITION:
    return 'waiting to add to next round';

  default:
    return 'bad player state';
  }
}

NETDATA.NEEDS_TABLE_BITMASK = (NETDATA.NEWCONN | NETDATA.CLIENT_EXITED | NETDATA.UPDATE_TABLE
  | NETDATA.DEAL);

NETDATA.NEEDS_PLAYER_BITMASK = (NETDATA.YOUR_PLAYER | NETDATA.NEW_PLAYER | NETDATA.CUR_PLAYERS
  | NETDATA.PLAYER_LEFT | NETDATA.PLAYER_ACTION | NETDATA.PLAYER_TURN | NETDATA.UPDATE_PLAYER
  | NETDATA.CUR_HAND | NETDATA.SHOW_HAND | NETDATA.DEAL);

NETDATA.NEEDS_ACTION_BITMASK = (NETDATA.ALLIN | NETDATA.BET | NETDATA.CALL | NETDATA.CHECK
 | NETDATA.FOLD | NETDATA.RAISE);

NETDATA.NEEDS_BITMASK = (NETDATA.NEEDS_TABLE_BITMASK | NETDATA.NEEDS_PLAYER_BITMASK);

NETDATA.needsTable = netData => {
  return netData.Response ? !!(netData.Response & NETDATA.NEEDS_TABLE_BITMASK) :
    !!(netData.Request & NETDATA.NEEDS_TABLE_BITMASK);
};

NETDATA.needsPlayer = netData => {
  return netData.Response ? !!(netData.Response & NETDATA.NEEDS_PLAYER_BITMASK) :
    !!(netData.Request & NETDATA.NEEDS_PLAYER_BITMASK);
};

export const TABLE_LOCK = {
  NONE:       0,
  PLAYERS:    1,
  SPECTATORS: 2,
  ALL:        3,
};

const TABLE_LOCK_NAME = [
  'none',
  'player lock',
  'spectator lock',
  'player & spectator lock',
];

TABLE_LOCK.toString = (lock) => {
  return TABLE_LOCK_NAME[lock] ?? 'invalid table lock';
};

export const TABLE_STATE = {
  NOT_STARTED: 0,

  PREFLOP: 1,
  FLOP:    2,
  TURN:    3,
  RIVER:   4,

  ROUNDS:        5,
  PLAYER_RAISED: 6,
  DONE_BETTING:  7,

  SHOW_HANDS: 8,
  SPLIT_POT:  9,
  ROUND_OVER: 10,
  NEW_ROUND:  11,
  GAME_OVER:  12,
  RESET:      13,
};

const TABLE_STATE_NAME = [
  "NOT_STARTED",
  "PREFLOP",
  "FLOP", "TURN", "RIVER",

  "ROUNDS", "PLAYER_RAISED", "DONE_BETTING",

  "SHOW_HANDS",
  "SPLIT_POT",
  "ROUND_OVER", "NEW_ROUND", "GAME_OVER",
  "RESET",
];

TABLE_STATE.toString = (state) => {
  return TABLE_STATE_NAME[state]?.toLowerCase() ?? 'invalid table state';
};

export function NewClient(settings) {
  const { RoomName, Name, Password, TableLock, TablePass } = settings;

  const haveAdminSettings = (
    RoomName !== undefined || TableLock !== undefined || TablePass !== undefined
  );

  return {
    Settings: {
      Name,
      Password,

      Admin: haveAdminSettings ? ({
        RoomName,
        Lock: TableLock,
        Password: TablePass
      }) : null,
    },
  };
}

export class NetData {
  constructor(client, request, msg = "", table = null) {
    this.Client = client;
    this.Request = BigInt(request);
    this.Msg = String(msg);
    this.Table = table;
  }

  toJSON() {
    return {
      Client: this.Client,
      Request: this.Request,
      Msg: String(this.Msg),
      Table: this.Table,
    };
  }

  toJSONStr() {
    return JSON.stringify(this.toJSON());
  }

  toMsgPack() {
    return encode(this.toJSON(), { useBigInt64: true });
  }
}

export function cardToImagePath(card) {
  const nameAndSuit = card.FullName.split(" of ");
  const name = nameAndSuit[0];
  const suit = nameAndSuit[1].charAt(0).toUpperCase() + nameAndSuit[1].slice(1);

  return `/cards/card${suit}${name}.png`;
}
