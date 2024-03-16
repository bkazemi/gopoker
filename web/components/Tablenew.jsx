import React, { useEffect, useState, useRef, useCallback, useContext } from 'react';

import { useRouter } from 'next/router';
import Image from 'next/image';
import { DM_Mono, VT323 } from 'next/font/google';
import dynamic from 'next/dynamic';

const dmMono = DM_Mono({ subsets: [ 'latin', 'latin-ext' ], weight: '500' });
const vt323 = VT323({ subsets: ['latin', 'latin-ext', 'vietnamese'], weight: '400' });

import cx from 'classnames';

import { NETDATA, NetData, NetDataToString, TABLE_LOCK, TABLE_STATE } from '@/lib/libgopoker';

import { GameContext } from '@/GameContext';
import TableModal from '@/components/TableModal';
import TableCenter from '@/components/TableCenter';
import PlayerTableItems from '@/components/PlayerTableItems';
import Player from '@/components/Player';
import Chat from '@/components/Chat';

const Header = dynamic(() => import('@/components/Header'), {
  ssr: false,
});

import styles from '@/styles/Tablenew.module.css';

const PlayerList = React.memo(({
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
        .filter(client => client.Player.TablePos % 4 === sideNum)
        .map((client, idx) => {
          //console.log(`${side} inner: ${!!innerTableItem} map: idx: ${idx} name: ${c.Name}`)
          const isYourPlayer = client.ID && client.ID === yourClient?.ID;
          //if (isYourPlayer)
          //  console.log(`PlayerList: yourPlayer found: id: ${client.ID}`);

          if (side === 'left' || side === 'right')
            gridRow = (~~(client.Player.TablePos / 4) % 3) + 1; // modulo 3 is max number
                                                                // of players on a given side
          else
            gridCol = (~~(client.Player.TablePos / 4) % 3) + 1;

          //console.log(`PlayerList: sideNum: ${sideNum} .TablePos: ${client.Player.TablePos} gridRow: ${gridRow} gridCol: ${gridCol}`)

          return innerTableItem
            ? <PlayerTableItems
                key={idx}
                {...{client, isYourPlayer, dealerAndBlinds, side, gridRow, gridCol, tableState}}
              />
            : <Player
                key={idx}
                {...{client, side, tableState, curPlayer, playerHead, gridRow, gridCol,
                     isYourPlayer, dealerAndBlinds, keyPressed, socket}}
              />
      })
    }
  </>);
});

PlayerList.displayName = 'PlayerList';

const nullPlayer = {
  Name: 'vacant seat',
  Action: {Action: NETDATA.VACANT_SEAT},
  ChipCount: 0,
};

const nullClient = {
  Player: nullPlayer,
  Name: 'vacant seat',
  _ID: 'vacant',
};

const nullPot = {
  Total: 0,
};

export default function Tablenew({ socket, connStatus, netData, setShowGame }) {
  //const [isPaused, setIsPaused] = useState(false);

  const {gameOpts, setGameOpts} = useContext(GameContext);

  // need this ref so that server response useEffect doesn't trigger when router changes
  const router = useRouter();
  const routerRef = useRef(router);

  const [yourClient, setYourClient] = useState(null);
  const yourClientID = useRef(null);
  const [isAdmin, setIsAdmin] = useState(false);
  const [isSpectator, setIsSpectator] = useState(gameOpts?.websocketOpts?.Client?.Settings?.IsSpectator);
  const [numSeats, setNumSeats] = useState(netData.Table?.NumSeats || 0);
  const [numPlayers, setNumPlayers] = useState(netData.Table?.NumPlayers || 0);
  const [numConnected, setNumConnected] = useState(netData.Table?.NumConnected || 0);
  const [chatMsgs, setChatMsgs] = useState([]);
  const [community, setCommunity] = useState([]);

  const [mainPot, setMainPot] = useState(netData.Table?.MainPot || nullPot);

  const [players, setPlayers] = useState(
    Array.from({length: netData.Table?.NumSeats || 0}, (_, idx) => ({
      ...nullClient,
      Player: {...nullPlayer, TablePos: idx}
    }))
  );
  const [curPlayer, setCurPlayer] = useState(null);
  const [playerHead, setPlayerHead] = useState(null);

  const [dealer, setDealer] = useState(netData.Table?.Dealer || nullPlayer);
  const [smallBlind, setSmallBlind] = useState(netData.Table?.SmallBlind || nullPlayer);
  const [bigBlind, setBigBlind] = useState(netData.Table?.BigBlind || nullPlayer);

  const [tablePass, setTablePass] = useState(netData.Table?.Password || "");
  const [tableLock, setTableLock] = useState(netData.Table?.Lock || TABLE_LOCK.NONE);
  const [tableState, setTableState] = useState(netData.Table?.State || TABLE_STATE.NOT_STARTED);

  // settings form
  const [settingsFormData, setSettingsFormData] = useState(null);

  // modal state
  const [modalType, setModalType] = useState('');
  const [modalOpen, setModalOpen] = useState(false);
  const [modalTxt, setModalTxt] = useState([]);

  const chatInputRef = useRef(null);

  useEffect(() => {
    switch (connStatus) {
    case 'rc':
      setModalType('reconnect');
      setModalTxt(['reconnecting...']);
      setModalOpen(true);
      break;
    case 'closed':
      setModalType('preGame');
      setModalTxt(['could not reconnect. connection closed']);
      setModalOpen(true);
      break;
    }
  }, [connStatus]);

  useEffect(() => {
    routerRef.current = router
  }, [router]);

  // keyboard event state
  const [keyPressed, setKeyPressed] = useState('');

  // keyboard action shortcuts
  useEffect(() => {
    if (modalOpen)
      return;

    const handleKeyDown = (event) => {
      if (chatInputRef?.current?.contains(document.activeElement))
        return;
      setKeyPressed(event.key);
    };

    const handleKeyUp = (event) => {
      if (chatInputRef?.current?.contains(document.activeElement))
        return;

      setKeyPressed('');
    };

    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('keyup', handleKeyUp);

    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('keyup', handleKeyUp);
    };
  }, [chatInputRef, modalOpen]);

  const updateRoom = useCallback((client) => {
    const router = routerRef.current;

    if (client.Settings?.Admin?.RoomName) {
      const newPath = `/room/${client.Settings.Admin.RoomName}`;
      if (newPath !== routerRef.asPath) {
        console.log(`newPath: ${newPath} router.asPath: ${router.asPath}`);
        console.log('replacing URL with:', newPath);
        router.replace(newPath);
      }
    }
  }, [routerRef]);

  const updateTable = useCallback((netData) => {
    setMainPot(netData.Table.MainPot);
    setCommunity(netData.Table.Community);
    setDealer(netData.Table.Dealer || nullClient);
    setSmallBlind(netData.Table.SmallBlind || nullClient);
    setBigBlind(netData.Table.BigBlind || nullClient);
    setTablePass(netData.Table.Password);
    setTableLock(netData.Table.Lock);
    setNumSeats(netData.Table.NumSeats);
    setTableState(netData.Table.State);
  }, []);

  const updatePlayer = useCallback((client, nullClient) => {
    if (!client.ID) {
      console.error('updatePlayer(): client without ID:', client);
      return;
    }

    setPlayers(players => {
      const pIdx = players.findIndex(c => c.ID === client.ID);
      if (pIdx !== -1) {
        const nullClientWithPos = nullClient ?
          {...nullClient,
           Player: {
            ...nullPlayer,
            TablePos: client.Player?.TablePos ?? players[pIdx].Player.TablePos // ELIMINATED resp does not include Player field
          }}
          : undefined;
        console.log(`updating ${players[pIdx].Name} to`, nullClientWithPos ?? client);
        const newClients = [...players];
        newClients[pIdx] = nullClientWithPos ?? client;
        return newClients;
      } else {
        console.error(`updatePlayer(): couldn't find ${client.ID} ${client.Player?.Name} in players array, pIdx: ${pIdx}`);
        console.log(players);
        return players;
      }
    });
  }, []);

  useEffect(() => {
    console.log('yourClient is ', yourClient);
    yourClientID.current = yourClient?.ID;
  }, [yourClient]);

  useEffect(() => {
    console.log('DSB: ', dealer, smallBlind, bigBlind);
  }, [dealer, smallBlind, bigBlind]);

  useEffect(() => {
  if (netData.Response) {
    console.log(`%cresp: ${NetDataToString(netData.Response)} (${netData.Response})`, "background-color:red;color:white;padding:5px;font-size:1.3rem");
    if (NETDATA.needsTable(netData) && !netData.Table)
      console.error('table needed but netData.Table is null');
    if (NETDATA.needsPlayer(netData) && (!netData.Client?.Player || !netData.Client?.ID))
      console.error(`needsPlayers(): player obj found ? ${!!netData.Client?.Player} ID ? ${!!NETDATA.Client?.ID}`);

    switch (netData.Response) {
    case NETDATA.NEWCONN: // FIXME: racy
      if (netData.Client) {
        if (!netData.Msg)
          console.error('newconn: no privID sent by server');
        netData.Client.privID = netData.Msg;
        window.privID = netData.Msg;
        setYourClient(netData.Client);
      }
      setNumConnected(netData.Table.NumConnected);
      if (netData.Table)
          updateTable(netData);
      break;
    case NETDATA.CLIENT_EXITED:
      if (netData.Client?.ID !== yourClientID.current) // XXX
        setChatMsgs(msgs => [...msgs, `<${netData.Client.Name} id: ${netData.Client.ID}> left the room`]);
      setNumConnected(netData.Table.NumConnected);
      break;
    case NETDATA.CHAT_MSG:
      console.log(`chatmsg: ${netData.Msg}`);
      setChatMsgs(msgs => [...msgs, netData.Msg]);
      break;
    case NETDATA.ROOM_SETTINGS:
      updateRoom(netData.Client);
      break;
    case NETDATA.CLIENT_SETTINGS:
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
    case NETDATA.PLAYER_RECONNECTING:
      netData.Client.Player.isDisconnected = true;
      updatePlayer(netData.Client);
      break;
    case NETDATA.PLAYER_RECONNECTED:
      netData.Client.Player.isDisconnected = false;
      updatePlayer(netData.Client);
      if (netData.Client.ID === yourClientID.current)
        setModalOpen(false);

      break;
    case NETDATA.PLAYER_LEFT: {
      console.log(`player left: id: ${netData.Client.ID} n: ${netData.Client.Player.Name}`);
      updatePlayer(netData.Client, nullClient);

      setChatMsgs(msgs => [...msgs, `<server-msg> ${netData.Client.Player.Name} left the table`]);
      setNumPlayers(netData.Table.NumPlayers);

      if (netData.Client.ID === yourClientID.current)
        setIsAdmin(false);

      break;
    }
    case NETDATA.MAKE_ADMIN:
      setIsAdmin(true);
      break;
    case NETDATA.DEAL:
      //setTableState(netData.Table.State);
      //setCommunity(netData.Table.Community);
      updatePlayer(netData.Client);
      break;
    case NETDATA.PLAYER_ACTION:
      updatePlayer(netData.Client);
      updateTable(netData);
      break;
    case NETDATA.PLAYER_HEAD:
      setPlayerHead(netData.Client);
      break;
    case NETDATA.PLAYER_TURN:
      setCurPlayer(netData.Client);
      break;
    case NETDATA.UPDATE_PLAYER:
      updatePlayer(netData.Client)
      break;
    case NETDATA.UPDATE_TABLE:
      updateTable(netData);
      break;
    case NETDATA.CUR_HAND:
      updatePlayer(netData.Client);
      break;
    case NETDATA.SHOW_HAND:
      updatePlayer(netData.Client);
      break;
    case NETDATA.ROUND_OVER:
      updateTable(netData);
      setModalType('');
      setModalTxt(arr => [...arr, netData.Msg]);
      setModalOpen(true);
      break;
    case NETDATA.RESET:
      if (netData.Client)
        updatePlayer(netData.Client);
      updateTable(netData);
      setPlayerHead(null);
      setCurPlayer(null);
      break;
    case NETDATA.ELIMINATED:
      if (netData.Client.ID === yourClientID.current) {
        setIsAdmin(false);
        setModalType('');
        setModalTxt(arr => [...arr, 'you have been eliminated']);
        setModalOpen(true);
      }
      // XXX: sometimes an UPDATE_PLAYER is being processed after
      //      PLAYER_LEFT, causing the players array to retain
      //      the eliminated player. if so, we will always remove them again
      //      from here for now.
      updatePlayer(netData.Client, nullClient);
      setChatMsgs(msgs => [...msgs, netData.Msg]);
      break;
    case NETDATA.FLOP:
    case NETDATA.TURN:
    case NETDATA.RIVER:
      setCommunity(netData.Table.Community);
      updateTable(netData);
      // we need to pause when there are new community cards
      // because for example when curPlayers are all in, the
      // server loops thru all the rounds automatically.
      // NOTE: this isn't really feasible. i've added this to
      // the backend for now.
      /*setIsPaused(true);
      setTimeout(() => {
        setIsPaused(false);
      }, 3000)*/
      break;
    case NETDATA.BAD_REQUEST:
    case NETDATA.SERVER_MSG:
      if (netData.Msg.startsWith('failed to reconnect')) {
        setModalTxt([netData.Msg]);
        setModalType('preGame');
      } else {
        setModalTxt(arr => [...arr, netData.Msg]);
        setModalType('');
      }
      setModalOpen(true);
      break;
    case NETDATA.TABLE_LOCKED:
        setModalType('preGame');
        setModalTxt(arr => ['this table is locked']);
        setModalOpen(true);
        break;
    case NETDATA.BAD_AUTH:
      setModalType('preGame');
      setModalTxt(arr => ['your password was incorrect']);
      setModalOpen(true);
      break;
    case NETDATA.SERVER_CLOSED:
      setModalType('preGame');
      setModalTxt(['server closed']);
      setModalOpen(true);
      break;
    default:
      setModalType('preGame');
      setModalTxt([`bad response: ${netData.Response}`]);
      setModalOpen(true);
      break;
    }
  }
  // NOTE: _noShallowCompare is given a new uuid on every new server response
  // passed from Connect component. We do this to ensure this useEffect gets
  // retriggered in cases where say the same user sends the same chat message,
  // causing the shallow comparison of the netData object to be the same for some reason.
  // My only guess is that it is a useSWRSubscription optimization.
  //
  // updatePlayer, updateRoom, etc. don't actually trigger rerenders.
  }, [netData._noShallowCompare, yourClientID, updatePlayer, updateRoom, updateTable]);

  useEffect(() => {
    if (numSeats) {
      setPlayers(players => {
        const newPlayerArr = players.filter(p => !p._ID);
        while (newPlayerArr.length < numSeats) {
          console.log('numSeats ue: TablePos:', Math.max(0, newPlayerArr.length))
          newPlayerArr.push({
            ...nullClient,
            Player: {
              ...nullPlayer,
              TablePos: Math.max(0, newPlayerArr.length)
            }
          });
        }

        console.log('numSeats ue: players =>', newPlayerArr);

        return newPlayerArr;
      });
    }
  }, [numSeats]);

  useEffect(() => {console.log(`players isArray: ${Array.isArray(players)}`); console.log(players); console.log(`pl len: ${players.length}`)}, [players]);

  useEffect(() => {
    if (modalType)
      console.log(`modalType set to ${modalType}`);
  }, [modalType]);

  /*useEffect(() => {
    if (settingsFormData) {
      console.log('settingsFormData', settingsFormData);
      socket.send(settingsFormData.toMsgPack());
    }
  }, [settingsFormData]);*/

  useEffect(() => {
    if (gameOpts.settingsChange) {
      console.log('Tablenew gameOpts.websocketOpts ue:', gameOpts.websocketOpts);
      socket.send(gameOpts.websocketOpts.toMsgPack());
      setGameOpts(opts => ({...opts, settingsChange: false}));
    }
  }, [gameOpts.settingsChange, gameOpts.websocketOpts, setGameOpts, socket]);

  useEffect(() => {
    if (isAdmin && gameOpts.creatorToken)
      setGameOpts(opts => ({...opts, creatorToken: ''}));

    setGameOpts(opts => ({
      ...opts,
      isAdmin,
    }));
  }, [isAdmin, setGameOpts, gameOpts.creatorToken]);

  useEffect(() => {
    if (gameOpts?.websocketOpts?.Client?.Settings?.IsSpectator === true && !isSpectator) {
      socket?.send((new NetData(yourClient, NETDATA.PLAYER_LEFT)).toMsgPack());
      setIsSpectator(true);
    }
  }, [gameOpts?.websocketOpts?.Client?.Settings?.IsSpectator, isSpectator, yourClient, socket]);

  return (
    //!isPaused &&
    <>
    <TableModal
      {...{modalType, modalTxt, setModalTxt, modalOpen, setModalOpen, setShowGame}}
      setFormData={setSettingsFormData}
    />
    <div
      className={cx(
        styles.tableGrid,
        dmMono.className,
        gameOpts.isCompactRoom && styles.compactTableGrid
      )}
      id='tableGrid'
    >
      <Header isTableHeader={true} />
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
            {...{players, curPlayer, playerHead, yourClient, keyPressed, socket, tableState}}
            dealerAndBlinds={{ dealer, smallBlind, bigBlind }}
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
          {...{players, curPlayer, playerHead, yourClient, keyPressed, socket, tableState}}
          dealerAndBlinds={{ dealer, smallBlind, bigBlind }}
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
            {...{isAdmin, tableState, community, mainPot, yourClient, socket}}
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
          {...{players, curPlayer, playerHead, yourClient, keyPressed, socket, tableState}}
          dealerAndBlinds={{ dealer, smallBlind, bigBlind }}
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
            {...{players, curPlayer, playerHead, yourClient, keyPressed, socket, tableState}}
            dealerAndBlinds={{ dealer, smallBlind, bigBlind }}
            sideNum={0}
          />
        </div>
      </div>
    </div>
    <div
      className={styles.topContainer}
    >
      <div className={styles.tableInfo}>
        <div>
          <label>table info</label>
          <Image
            title='settings'
            src={'/settingsIcon.png'}
            height={35}
            width={35}
            alt={'<settings>'}
            onClick={() => {
              setModalType('settings');
              setModalOpen(true);
            }}
          />
          {
            !isSpectator &&
            <Image
              title='move to spectator'
              src={'/spectator.png'}
              height={35}
              width={35}
              alt={'<spectate>'}
              onClick={() => {
                setModalTxt(arr => [...arr, 'are you sure?']);
                setModalType('spectate');
                setModalOpen(true);
              }}
              style={{
                paddingRight: '5px'
              }}
            />
          }
          <Image
            title='quit game'
            src={'/quitGame.png'}
            height={35}
            width={35}
            alt={'<quit game>'}
            onClick={() => {
              setModalTxt(arr => [...arr, 'are you sure?']);
              setModalType('quit');
              setModalOpen(true);
            }}
            style={{ marginRight: '5px' }}
          />
        </div>
        <div className={cx(styles.tableInfoItems, vt323.className)}>
          <p>
            name: { yourClient?.Name }
            <br />
            <span style={{ fontStyle: 'italic' }}>
              id: { yourClient?.ID }
            </span>
          </p>
          <p># players: { numPlayers }</p>
          <p># connected: { String(numConnected) }</p>
          <p># open seats: { numSeats - numPlayers }</p>
          <p>password protected: { tablePass ? 'yes' : 'no' }</p>
          <p>table lock: { TABLE_LOCK.toString(tableLock) }</p>
          <p>status: { TABLE_STATE.toString(tableState) }</p>
        </div>
      </div>
      <Chat
        {...{yourClient, socket, chatInputRef}}
        msgs={chatMsgs}
      />
    </div>
  </>
  );
}
