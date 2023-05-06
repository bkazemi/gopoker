import React, {useEffect, useState, useRef, useCallback} from 'react';

import Image from 'next/image';

import cx from 'classnames';

import { NETDATA, NetData, TABLE_STATE } from '@/lib/libgopoker';

import TableModal from '@/components/TableModal';
import TableCenter from '@/components/TableCenter';
import PlayerTableItems from '@/components/PlayerTableItems';
import Player from '@/components/Player';
import Chat from '@/components/Chat';

import styles from '@/styles/Tablenew.module.css';

const PlayerList = ({
  players, curPlayer, playerHead, dealerAndBlinds, yourClient, sideNum, innerTableItem,
  tableState, keyPressed, socket
}) => {
  let side;
  sideNum === 0 && (side = 'bottom');
  sideNum === 1 && (side = 'left');
  sideNum === 2 && (side = 'top');
  sideNum === 3 && (side = 'right');

  let gridRow = 1;
  let gridCol = 1;

  return (<>
    {
      players
        .filter((_, idx) => idx % 4 === sideNum)
        .map((client, idx) => {
          //console.log(`${side} inner: ${!!innerTableItem} map: idx: ${idx} name: ${c.Name}`)
          const isYourPlayer = client.ID && client.ID === yourClient?.ID;
          if (isYourPlayer)
            console.log(`PlayerList: yourPlayer found: id: ${client.ID}`);

          if (side === 'left' || side === 'right')
            gridRow = idx+1;
          else
            gridCol = idx+1;

          return innerTableItem 
            ? <PlayerTableItems
                {...{client, isYourPlayer, dealerAndBlinds, side, gridRow, gridCol, tableState}}
              />
            : <Player
                {...{client, side, curPlayer, playerHead, gridRow, gridCol,
                     isYourPlayer, keyPressed, socket}}
              />
      })
    }
  </>);
};

const nullPlayer = {
  Name: 'vacant seat',
  Action: {Action: NETDATA.VACANT_SEAT},
  ChipCount: Infinity,
};

const nullClient = {
  Player: nullPlayer,
  Name: 'vacant seat',
  _ID: 'vacant',
};

const nullPot = {
  Total: 0,
};

export default function Tablenew({ socket, netData, setShowGame }) {
  const [yourClient, setYourClient] = useState(null);
  const [isAdmin, setIsAdmin] = useState(false);
  const [numSeats, setNumSeats] = useState(netData.Table?.NumSeats || 0);
  const [numPlayers, setNumPlayers] = useState(netData.Table?.NumPlayers || 0);
  const [numConnected, setNumConnected] = useState(netData.Table?.NumConnected || 0);
  const [chatMsgs, setChatMsgs] = useState([]);
  const [community, setCommunity] = useState([]);

  const [mainPot, setMainPot] = useState(netData.Table?.MainPot || nullPot);

  const [players, setPlayers] = useState(
    Array.from({length: netData.Table?.NumSeats || 0}, () => nullClient)
  );
  const [curPlayer, setCurPlayer] = useState(null);
  const [playerHead, setPlayerHead] = useState(null);

  const [dealer, setDealer] = useState(netData.Table?.Dealer || nullPlayer);
  const [smallBlind, setSmallBlind] = useState(netData.Table?.SmallBlind || nullPlayer);
  const [bigBlind, setBigBlind] = useState(netData.Table?.BigBlind || nullPlayer);

  const [tableState, setTableState] = useState(netData.Table?.State || TABLE_STATE.NOT_STARTED);

  // settings form
  const [settingsFormData, setSettingsFormData] = useState(null);

  // modal state
  const [modalType, setModalType] = useState('');
  const [modalOpen, setModalOpen] = useState(false);
  const [modalTxt, setModalTxt] = useState('');

  // keyboard event state
  const [keyPressed, setKeyPressed] = useState('');

  // keyboard action shortcuts
  useEffect(() => {
    const handleKeyDown = (event) => {
      setKeyPressed(event.key);
    };

    const handleKeyUp = (event) => {
      setKeyPressed('');
    };

    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('keyup', handleKeyUp);

    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('keyup', handleKeyUp);
    };
  }, []);

  const updateTable = useCallback(() => {
    setMainPot(netData.Table.MainPot);
    setCommunity(netData.Table.Community);
    setDealer(netData.Table.Dealer || nullClient);
    setSmallBlind(netData.Table.SmallBlind || nullClient);
    setBigBlind(netData.Table.BigBlind || nullClient);
    setTableState(netData.Table.State);
  }, [netData]);

  const updatePlayer = useCallback((client, nullClient) => {
    console.log(players);
    const pIdx = players.findIndex(c => c.ID === client.ID);
    if (pIdx !== -1) {
      console.log(`updating ${players[pIdx].Name} to ${nullClient ?? client}`);
      setPlayers(c => {
        const newClients = [...c];
        newClients[pIdx] = nullClient ?? client; 
        return newClients;
      });
    } else {
      console.error(`updatePlayer(): couldn't find ${client.ID} ${client.Player?.Name} in players array, pIdx: ${pIdx}`); 
      console.log(players);
    }
  }, [players]);

  useEffect(() => {
    console.log(`yourClient is ${yourClient}`);
  }, [yourClient]);

  useEffect(() => {
    console.log('DSB: ', dealer, smallBlind, bigBlind);
  }, [dealer, smallBlind, bigBlind]);

  useEffect(() => {
    if (numSeats) {
      ;
    }
  }, [numSeats]);

  useEffect(() => {
  if (netData.Response) {
    if (NETDATA.needsTable(netData) && !netData.Table)
      console.error('table needed but netData.Table is null');
    if (NETDATA.needsPlayer(netData) && (!netData.Client?.Player || !netData.Client?.ID))
      console.error(`needsPlayers(): player obj found ? ${!!netData.Client?.Player} ID ? ${!!NETDATA.Client?.ID}`);

    switch (netData.Response) {
    case NETDATA.NEWCONN:
      if (!yourClient?.ID)
        setYourClient(netData.Client);
      setNumConnected(netData.Table.NumConnected);
    case NETDATA.CLIENT_EXITED:
      setNumConnected(netData.Table.NumConnected);
      break;
    case NETDATA.CHAT_MSG:
      console.log(`chatmsg: ${netData.Msg}`);
      setChatMsgs(msgs => [...msgs, netData.Msg]);
      break;
    case NETDATA.CLIENT_SETTINGS:
      console.log(`client settings`);
      setYourClient(netData.Client);
      updatePlayer(netData.Client);
      break;
    case NETDATA.YOUR_PLAYER: {
      if (netData.Table)
        setNumPlayers(netData.Table.NumPlayers);

      setYourClient(netData.Client);
      setPlayers(clients => {
        const newClients = [...clients];
        const vacantSeatIdx = clients.findIndex(c => c.Name === nullClient.Name)
        if (vacantSeatIdx !== -1) {
          console.log(`yp: setting c[${vacantSeatIdx}] => ${netData.Client.Name}`)
          newClients[vacantSeatIdx] = netData.Client;
        } else {
          console.error(`your_player: couldnt find vacant seat idx ${vacantSeatIdx} [${clients.map(c => c.Name)}] nc.c ${nullClient.Name}`);
        }

        return newClients;
      });

      break;
    }
    case NETDATA.NEW_PLAYER:
    case NETDATA.CUR_PLAYERS:
      setNumPlayers(netData.Table.NumPlayers);

      console.log(`${netData.Response === NETDATA.NEW_PLAYER ? 'new_players' : 'cur_players'} recv p: ${netData.Client.Name}`);

      setPlayers(clients => {
        const newClients = [...clients];
        const vacantSeatIdx = clients.findIndex(c => c.Name === nullClient.Name);
        if (vacantSeatIdx !== -1) {
          console.log(`nc: setting c[${vacantSeatIdx}] => ${netData.Client.Name}`)
          newClients[vacantSeatIdx] = netData.Client;
        } else
          console.error('new_player or cur_players: couldnt find vacant seat idx');

        return newClients;
      });
      break;
    case NETDATA.PLAYER_LEFT: {
      console.log(`player left: id: ${netData.Client.ID} n: ${netData.Client.Player.Name}`);
      updatePlayer(netData.Client, nullClient);

      setChatMsgs(msgs => [...msgs, `<server-msg> ${netData.Client.Player.Name} left the table`]);
      setNumPlayers(netData.Table.NumPlayers);
      break;
    }
    case NETDATA.MAKE_ADMIN:
      console.log('make admin');
      setIsAdmin(true);
      break;
    case NETDATA.DEAL:
      console.log('deal');
      //setTableState(netData.Table.State);
      //setCommunity(netData.Table.Community);
      updatePlayer(netData.Client);
      break;
    case NETDATA.PLAYER_ACTION:
      console.log('player action');
      updatePlayer(netData.Client);
      updateTable();
      break;
    case NETDATA.PLAYER_HEAD:
      console.log('player head');
      setPlayerHead(netData.Client);
      break;
    case NETDATA.PLAYER_TURN:
      console.log('player turn');
      setCurPlayer(netData.Client);
      break;
    case NETDATA.UPDATE_PLAYER:
      console.log('update player');
      updatePlayer(netData.Client)
      break;
    case NETDATA.UPDATE_TABLE:
      console.log('update table');
      updateTable();
      break;
    case NETDATA.CUR_HAND:
      console.log('cur hand');
      updatePlayer(netData.Client);
      break;
    case NETDATA.SHOW_HAND:
      console.log('show hand');
      break;
    case NETDATA.ROUND_OVER:
      console.log('round over');
      updateTable();
      setModalType('');
      setModalTxt(netData.Msg);
      setModalOpen(true);
      break;
    case NETDATA.RESET:
      console.log('reset');
      updateTable();
      break;
    case NETDATA.ELIMINATED:
      console.log('elim');
      if (netData.Client.ID === yourClient.ID) {
        setModalType('');
        setModalTxt('you have been eliminated');
        setModalOpen(true);
      }
      setChatMsgs(msgs => [...msgs, netData.Msg]);
      break;
    case NETDATA.FLOP:
    case NETDATA.TURN:
    case NETDATA.RIVER:
      console.log('flop, turn or river');
      setCommunity(netData.Table.Community);
      updateTable();
      break;
    case NETDATA.BAD_REQUEST:
    case NETDATA.SERVER_MSG:
      setModalType('');
      setModalTxt(netData.Msg);
      setModalOpen(true);
      break;
    case NETDATA.TABLE_LOCKED:
    case NETDATA.BAD_AUTH:
      setModalType('preGame');
      setModalTxt('lock or bad auth');
      setModalOpen(true);
      break;
    case NETDATA.SERVER_CLOSED:
      setModalType('preGame');
      setModalTxt('server closed');
      setModalOpen(true);
      break;
    default:
      setModalType('preGame');
      setModalTxt(`bad response: ${netData.Response}`);
      setModalOpen(true);
      break;
    }
  }
  }, [netData.ShallowThis]);

  useEffect(() => {console.log(`players isArray: ${Array.isArray(players)}`); console.log(players); console.log(`pl len: ${players.length}`)}, [players]);

  useEffect(() => {
    if (modalType)
      console.log(`modalType set to ${modalType}`);
  }, [modalType]);

  useEffect(() => {
    if (settingsFormData) {
      console.log('settingsFormData', settingsFormData);
      socket.send(settingsFormData.toMsgPack());
    }
  }, [settingsFormData]);

  return (
    <>
    <TableModal
      {...{modalType, modalTxt, modalOpen, setModalOpen, setShowGame}}
      setFormData={setSettingsFormData}
    />
    <div className={styles.tableGrid} id='tableGrid'>
      <div
        id='topPlayers'
        className={cx(
          styles.tableGridItem,
          styles.topPlayersContainer
        )}
      >
        <div
          className={cx(
            styles.tableGridItem,
            styles.topPlayers
          )}
        >
        {/* TOP-SIDE PLAYERS */}
          <PlayerList
            {...{players, curPlayer, playerHead, yourClient, keyPressed, socket}}
            sideNum={2}
          />
        </div>
      </div>
      <div
        id='leftPlayers'
        className={cx(
          styles.tableGridItem,
          styles.leftPlayers
        )}
      >
        {/* LEFT-SIDE PLAYERS */}
        <PlayerList
          {...{players, curPlayer, playerHead, yourClient, keyPressed, socket}}
          sideNum={1}
        />
      </div>
      <div className={styles.innerTableGrid}>
        <div
          id='tableTop'
          className={cx(
            styles.innerTableGridItem,
            styles.topPlayers
          )}
        >
          {/* TOP OF TABLE */}
          <PlayerList
            {...{players, curPlayer, playerHead, yourClient, keyPressed, tableState}}
            dealerAndBlinds={{ dealer, smallBlind, bigBlind }}
            sideNum={2}
            innerTableItem={true}
          />
        </div>
        <div
          id='tableLeft'
          className={cx(
            styles.innerTableGridItem,
            styles.leftPlayers
          )}
        >
          {/* LEFT-SIDE OF TABLE */}
          <PlayerList
            {...{players, curPlayer, playerHead, yourClient, tableState}}
            dealerAndBlinds={{ dealer, smallBlind, bigBlind }}
            sideNum={1}
            innerTableItem={true}
          />
        </div>
        <div
          id='tableCenter'
          className={cx(
            styles.innerTableGridItem,
            styles.center
          )}
        >
          {/* DEAD-CENTER OF TABLE */}
          <TableCenter
            {...{isAdmin, tableState, community, yourClient, socket}}
          />
        </div>
        <div
          id='tableRight'
          className={cx(
            styles.innerTableGridItem,
            styles.rightPlayers
          )}
        >
          {/* RIGHT-SIDE OF TABLE */}
          <PlayerList
            {...{players, curPlayer, playerHead, yourClient, tableState}}
            dealerAndBlinds={{ dealer, smallBlind, bigBlind }}
            sideNum={3}
            innerTableItem={true}
          />
        </div>
        <div
          id='tableBottom'
          className={cx(
            styles.innerTableGridItem,
            styles.bottomPlayers
          )}
        >
          {/* BOTTOM OF TABLE */}
          <PlayerList
            {...{players, curPlayer, playerHead, yourClient, tableState}}
            dealerAndBlinds={{ dealer, smallBlind, bigBlind }}
            sideNum={0}
            innerTableItem={true}
          />
        </div>
      </div>
      <div
        id='rightPlayers'
        className={cx(
          styles.tableGridItem,
          styles.rightPlayers
        )}
      >
        {/* RIGHT-SIDE PLAYERS */}
        <PlayerList
          {...{players, curPlayer, playerHead, yourClient, keyPressed, socket}}
          sideNum={3}
        />
      </div>
      <div
        id='bottomPlayers'
        className={cx(
          styles.tableGridItem,
          styles.bottomPlayersContainer
        )}
      >
        <div
          className={cx(
            styles.tableGridItem,
            styles.bottomPlayers
          )}
        >
          {/* BOTTOM-SIDE PLAYERS */}
          <PlayerList 
            {...{players, curPlayer, playerHead, yourClient, keyPressed, socket}}
            sideNum={0}
          />
        </div>
      </div>
    </div>
    <div className={styles.topContainer}>
      <div className={styles.tableInfo}>
        <div>
          <label>table info</label>
          <Image
            src={'/settingsIcon.png'}
            height={35}
            width={35}
            alt={'<settings>'}
            onClick={() => {
              setModalType('settings');
              setModalOpen(true);
            }}
          />
         <Image
          src={'/quitGame.png'}
          height={35}
          width={35}
          alt={'<quit game>'}
          onClick={() => {
            setModalTxt('are you sure?');
            setModalType('quit');
            setModalOpen(true);
          }}
          style={{ marginRight: '5px' }}
        />
        </div>
        <p># players: { numPlayers }</p>
        <p># connected: { String(numConnected) }</p>
        <p># open seats: { numSeats - numPlayers }</p>
        <p>pot: { mainPot.Total.toLocaleString() }</p>
        <p>dealer: { String(dealer.Player?.Name || '') }</p>
        <p>small blind: { String(smallBlind.Player?.Name || '') }</p>
        <p>big blind: { String(bigBlind.Player?.Name || '') }</p>
        <p>status: { TABLE_STATE.toString(tableState) }</p>
      </div>
      <Chat
        {...{yourClient, socket}}
        msgs={chatMsgs}/>
    </div>
  </>
  );
}
