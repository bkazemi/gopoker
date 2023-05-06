import React, {useEffect, useState} from 'react';

import { NETDATA, TABLE_STATE } from '@/lib/libgopoker';

import Player from '@/components/Player';

import styles from '@/styles/Table.module.css';

export default function Table({ NetData }) {
  const nullPlayer = {
    Name: 'none',
  };

  const nullPot = {
    Total: 0,
  };

  const [yourClient, setYourClient] = useState(null);
  const [numSeats, setNumSeats] = useState(NetData.Table?.NumSeats || 0);
  const [numPlayers, setNumPlayers] = useState(NetData.Table?.NumPlayers || 0);
  const [numConnected, setNumConnected] = useState(NetData.Table?.NumConnected || 0);

  const [mainPot, setMainPot] = useState(NetData.Table?.MainPot || nullPot);

  const [players, setPlayers] = useState([]);

  const [dealer, setDealer] = useState(NetData.Table?.Dealer || nullPlayer);
  const [smallBlind, setSmallBlind] = useState(NetData.Table?.SmallBlind || nullPlayer);
  const [bigBlind, setBigBlind] = useState(NetData.Table?.BigBlind || nullPlayer);

  const [tableState, setTableState] = useState(NetData.Table?.State || TABLE_STATE.NOT_STARTED);

  useEffect(() => {
    console.log(`yourClient is ${yourClient}`);
  }, [yourClient]);

  /*useEffect(() => {
    const playerSides = [
      document.getElementById('bottomPlayers'),
      document.getElementById('leftPlayers'),
      document.getElementById('topPlayers'),
      document.getElementById('rightPlayers'),
    ];

    if (players) {
      players.forEach((p, idx) => {
        const playerSide = playerSides[idx % playerSides.length];
        const playerIdx = ~~(idx / playerSides.length);
        if (playerIdx >= playerSide.length)
          playerSide.appendChild(<Player player={p} />)
        else
          playerSide.insertBefore(<Player player={p} />, playerSide.children[playerIdx]);
      });
    }
  }, [players]);*/

  useEffect(() => {
  if (NetData.Response) {
    if (NETDATA.needsTable(NetData) && !NetData.Table)
      console.error('table needed but NetData.Table is null');
    if (NETDATA.needsPlayer(NetData) && (!NetData.Client?.Player || !NetData.Client?.ID))
      console.error(`needsPlayers(): player obj found ? ${!!NetData.Client?.Player} ID ? ${!!NETDATA.Client?.ID}`);

    switch (NetData.Response) {
    case NETDATA.NEWCONN:
      if (!yourClient?.ID)
        setYourClient(NetData.Client);
    case NETDATA.CLIENT_EXITED:
      setNumConnected(NetData.Table.NumConnected);
      break;
    case NETDATA.CHAT_MSG:
      console.log(`chatmsg: ${NetData.Msg}`);
      break;
    case NETDATA.CLIENT_SETTINGS:
      setYourClient(NetData.Client);
      break;
    case NETDATA.YOUR_PLAYER:
      if (NetData.Table)
        setNumPlayers(NetData.Table.NumPlayers);

      setYourClient(NetData.Client);
      setPlayers(clients => [...clients, NetData.Client]);
      break;
    case NETDATA.NEW_PLAYER:
    case NETDATA.CUR_PLAYERS:
      setNumPlayers(NetData.Table.NumPlayers);

      setPlayers(clients => [...clients, NetData.Client]);
      break;
    case NETDATA.PLAYER_LEFT:
      console.log(`player left: ${NetData.Client.Player.Name}`);
      setNumPlayers(NetData.Table.NumPlayers);
      break;
    case NETDATA.MAKE_ADMIN:
      console.log('make admin');
      break;
    case NETDATA.DEAL:
      console.log('deal');
      break;
    case NETDATA.PLAYER_ACTION:
      console.log('player action');
      setTableState(NetData.Table.State);
      break;
    case NETDATA.PLAYER_HEAD:
      console.log('player head');
      break;
    case NETDATA.PLAYER_TURN:
      console.log('player turn');
      break;
    case NETDATA.UPDATE_PLAYER:
      console.log('update player');
      break;
    case NETDATA.UPDATE_TABLE:
      console.log('update table');
      break;
    case NETDATA.CUR_HAND:
      console.log('cur hand');
      break;
    case NETDATA.SHOW_HAND:
      console.log('show hand');
      break;
    case NETDATA.ROUND_OVER:
      console.log('round over');
      break;
    case NETDATA.RESET:
      console.log('reset');
      break;
    case NETDATA.ELIMINATED:
      console.log('elim');
      break;
    case NETDATA.FLOP:
    case NETDATA.TURN:
    case NETDATA.RIVER:
      console.log('flop, turn or river');
      break;
    case NETDATA.BAD_REQUEST:
    case NETDATA.SERVER_MSG:
      alert(NetData.Msg);
      break;
    case NETDATA.TABLE_LOCKED:
    case NETDATA.BAD_AUTH:
      alert('lock or bad auth');
    case NETDATA.SERVER_CLOSED:
      alert('server closed');
    default:
      alert(`bad response: ${NetData.Response}`);
    }
  }
  }, [NetData]);

  useEffect(() => {console.log(`players isArray: ${Array.isArray(players)}`); console.log(players); console.log(`pl len: ${players.length}`)}, [players]);

  return (
    <>
    <div className={styles.tableInfo}>
      <p># players: {numPlayers}</p>
      <p># connected: {numConnected}</p>
      <p># open seats: {numSeats - numPlayers}</p>
      <p>pot: {mainPot.Total}</p>
      <p>dealer: {dealer.Name}</p>
      <p>small blind: {smallBlind.Name}</p>
      <p>big blind: {bigBlind.Name}</p>
      <p>status: {tableState}</p>
    </div>
    <div className={styles.playerSpace}>
      <div id='topPlayers' className={styles.topPlayers}>
        {/* TOP-SIDE PLAYERS */}
        {players
          .filter((_, idx) => idx % 4 === 2)
          .map((c, idx) => {
            console.log(`topside map: idx: ${idx} name: ${c.Name}`)
            return <Player client={c} gridCol={idx+1} />
        })}
      </div>
      <div className={styles.middle}>
        <div id='leftPlayers' className={styles.leftPlayers}>
          {/* LEFT-SIDE PLAYERS */}
          {players
            .filter((_, idx) => idx % 4 === 1)
            .map((c, idx) => {
              console.log(`leftside map: idx: ${idx} name: ${c.Name}`)
              return <Player client={c} gridCol={idx+1} />
            })}
        </div>
        <div className={styles.tableSpace}>
          <div id='tableTop' className={styles.tableTop}>
            {/* TOP OF TABLE */}
          </div>
          <div className={styles.tableMiddle}>
            {/* MIDDLE OF TABLE */}
            <div id='tableLeft' className={styles.tableLeft}>
              {/* LEFT-SIDE OF TABLE */}
            </div>
            <div id='tableCenter' className={styles.tableCenter}>
              {/* DEAD-CENTER OF TABLE */}
            </div>
            <div id='tableRight' className={styles.tableRight}>
              {/* RIGHT-SIDE OF TABLE */}
            </div>
          </div>
          <div id='tableBottom' className={styles.tableBottom}>
            {/* BOTTOM OF TABLE */}
          </div>
        </div>
        <div id='rightPlayers' className={styles.rightPlayers}>
          {/* RIGHT-SIDE PLAYERS */}
          {
            players
              .filter((_, idx) => idx % 4 === 3)
              .map((c, idx) => {
                console.log(`rightside map: idx: ${idx} name: ${c.Name}`)
                return <Player client={c} gridCol={idx+1} />
            })
          }
        </div>
      </div>
      <div id='bottomPlayers' className={styles.bottomPlayers}>
        {/* BOTTOM-SIDE PLAYERS */}
        {
          players 
            .filter((_, idx) => idx % 4 === 0)
            .map((c, idx) => {
              console.log(`botside map: idx: ${idx} name: ${c.Name}`);
              return <Player client={c} gridCol={idx+1} />
          })
        }
      </div>
    </div>
  </>
  );
}
