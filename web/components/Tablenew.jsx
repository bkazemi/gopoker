import React, { useEffect, useState, useRef, useCallback, useContext } from 'react';

import { useRouter } from 'next/router';
import Image from 'next/image';
import { DM_Mono, VT323 } from 'next/font/google';
import dynamic from 'next/dynamic';

const dmMono = DM_Mono({ subsets: [ 'latin', 'latin-ext' ], weight: '500' });
const vt323 = VT323({ subsets: ['latin', 'latin-ext', 'vietnamese'], weight: '400' });

import { v4 as uuidv4 } from 'uuid';
import cx from 'classnames';

import { NETDATA, NetData, NetDataToString, TABLE_LOCK, TABLE_STATE } from '@/lib/libgopoker';
import useWindowMetrics from '@/lib/useWindowMetrics';

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
  players, curPlayer, curHand, playerHead, dealerAndBlinds, yourClient,
  isSpectator, sideNum, innerTableItem, tableState, keyPressed, socket
}) => {
  const side = ['bottom', 'left', 'top', 'right'][sideNum];

  return (<>
    {
      players
        .filter(client => client.Player.TablePos % 4 === sideNum)
        .map(client => {
          const isYourPlayer = client.ID && client.ID === yourClient?.ID;
          const gridOffset = (~~(client.Player.TablePos / 4) % 3) + 1;
          const gridRow = side === 'left' || side === 'right' ? gridOffset : 1;
          const gridCol = side === 'left' || side === 'right' ? 1 : gridOffset;

          return innerTableItem
            ? <PlayerTableItems
                key={client.ID || client._ID}
                {...{client, isYourPlayer, curHand, dealerAndBlinds, side, gridRow, gridCol, tableState}}
              />
            : <Player
                key={client.ID || client._ID}
                {...{client, yourClient, side, tableState, curPlayer, playerHead, gridRow, gridCol,
                     isYourPlayer, isSpectator, dealerAndBlinds, keyPressed, socket}}
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
};

const NewNullClient = (tablePos) => ({
  Player: {
    ...nullPlayer,
    TablePos: tablePos
  },
  Name: 'vacant seat',
  _ID: uuidv4(),
});

const nullPot = {
  Total: 0,
};

export default function Tablenew({ socket, connStatus, netData, setShowGame, roomIDRef }) {
  const {gameOpts, setGameOpts} = useContext(GameContext);
  const { innerWidth } = useWindowMetrics();

  // need this ref so that server response useEffect doesn't trigger when router changes
  const router = useRouter();
  const routerRef = useRef(router);

  const [yourClient, setYourClient] = useState(null);
  const yourClientRef = useRef(null);
  const [isAdmin, setIsAdmin] = useState(false);
  const [isSpectator, setIsSpectator] = useState(!!gameOpts?.websocketOpts?.Client?.Settings?.IsSpectator);
  const [numSeats, setNumSeats] = useState(netData.Table?.NumSeats || 0);
  const [numPlayers, setNumPlayers] = useState(netData.Table?.NumPlayers || 0);
  const [numConnected, setNumConnected] = useState(netData.Table?.NumConnected || 0);
  const [chatMsgs, setChatMsgs] = useState([]);
  const [community, setCommunity] = useState([]);

  const [mainPot, setMainPot] = useState(netData.Table?.MainPot || nullPot);

  const [players, setPlayers] = useState(
    Array.from({length: netData.Table?.NumSeats || 0}, (_, idx) => ({
      ...NewNullClient(idx),
    }))
  );
  const [curPlayer, setCurPlayer] = useState(null);
  const [curHand, setCurHand] = useState(null);
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
  const showModal = useCallback((nextModalType, nextModalTxt) => {
    setModalType(nextModalType);
    setModalTxt(nextModalTxt);
    setModalOpen(true);
  }, []);
  const applyYourClient = useCallback((nextClient) => {
    console.log('yourClient is ', nextClient);
    yourClientRef.current = nextClient;
    setYourClient(nextClient);
    setIsSpectator(!!nextClient?.Settings?.IsSpectator);
  }, []);
  const normalizePlayersForSeats = useCallback((seatCount) => {
    if (!seatCount)
      return;

    setPlayers(players => {
      const newPlayerArr = players.filter(p => !p._ID);
      const playerPosSet = new Set(newPlayerArr.map(c => c.Player.TablePos));

      console.log('numSeats ue: playerPosSet:', playerPosSet);

      let curTablePos = 0;
      while (newPlayerArr.length < seatCount) {
        console.log(`numSeats ue: curTablePos before map: ${curTablePos}`);
        while (playerPosSet.has(curTablePos))
          curTablePos++;
        console.log(`numSeats ue: curTablePos after map: ${curTablePos}`);
        newPlayerArr.push({
          ...(NewNullClient(curTablePos++)),
        });
      }

      console.log('numSeats ue: players =>', newPlayerArr);

      return newPlayerArr;
    });
  }, []);

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    switch (connStatus) {
    case 'rc':
      showModal('reconnect', ['reconnecting...']);
      break;
    case 'closed':
      showModal('preGame', ['could not reconnect. connection closed']);
      break;
    }
  }, [connStatus, showModal]);
  /* eslint-enable react-hooks/set-state-in-effect */

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

  const updateRoom = useCallback((roomSettings) => {
    const router = routerRef.current;

    if (router && roomSettings?.RoomName) {
      const newPath = `/room/${encodeURIComponent(roomSettings.RoomName)}`;
      if (newPath !== router.asPath) {
        console.log(`newPath: ${newPath} router.asPath: ${router.asPath}`);
        console.log('replacing URL with:', newPath);
        if (roomIDRef)
          roomIDRef.current = roomSettings.RoomName;
        setGameOpts(opts => ({ ...opts, roomRenamed: true }));
        router.replace({ pathname: newPath });
      }
    }
  }, [routerRef, roomIDRef, setGameOpts]);

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
          {...NewNullClient(
            client.Player?.TablePos ?? players[pIdx].Player.TablePos // ELIMINATED resp does not include Player field
          )} : undefined;
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
    console.log('DSB: ', dealer, smallBlind, bigBlind);
  }, [dealer, smallBlind, bigBlind]);

  /* eslint-disable react-hooks/set-state-in-effect */
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
        const nextClient = {
          ...netData.Client,
          privID: netData.Msg,
        };
        window.privID = netData.Msg;
        applyYourClient(nextClient);
      }
      setNumConnected(netData.Table.NumConnected);
      if (netData.Table)
          updateTable(netData);
      break;
    case NETDATA.CLIENT_EXITED:
      if (netData.Client?.ID !== yourClientRef.current?.ID) // XXX
        setChatMsgs(msgs => [...msgs, `<${netData.Client.Name} id: ${netData.Client.ID}> left the room`]);
      setNumConnected(netData.Table.NumConnected);
      break;
    case NETDATA.CHAT_MSG:
      console.log(`chatmsg: ${netData.Msg}`);
      setChatMsgs(msgs => [...msgs, netData.Msg]);
      break;
    case NETDATA.ROOM_SETTINGS:
      updateRoom(netData.RoomSettings);
      setGameOpts(opts => ({
        ...opts,
        roomSettings: netData.RoomSettings,
      }));
      break;
    case NETDATA.CLIENT_SETTINGS:
      applyYourClient(netData.Client);
      if (netData.Client.Player)
        updatePlayer(netData.Client);
      setGameOpts(opts => ({
        ...opts,
        websocketOpts: {
          ...opts.websocketOpts,
          Client: netData.Client,
        }
      }));
      break;
    case NETDATA.YOUR_PLAYER: {
      if (netData.Table)
        setNumPlayers(netData.Table.NumPlayers);

      applyYourClient(netData.Client);
      setPlayers(clients => {
        const newClients = [...clients];

        let vacantSeatIdx = newClients.findIndex(c => c.Player.TablePos === netData.Client.Player.TablePos);
        if (vacantSeatIdx === -1)
          vacantSeatIdx = clients.findIndex(c => c._ID);

        if (vacantSeatIdx !== -1) {
          if (!newClients[vacantSeatIdx]._ID)
            console.error(`yp: tried to set ${netData.Client.Name} to occupied seat: c[${vacantSeatIdx}]`);
          else {
            console.log(`yp: setting c[${vacantSeatIdx}] => ${netData.Client.Name}`)
            newClients[vacantSeatIdx] = netData.Client;
          }
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

      const pfx = netData.Response === NETDATA.NEW_PLAYER ? 'new_players' : 'cur_players';

      console.log(`${pfx} recv p: ${netData.Client.Name}`);

      setPlayers(clients => {
        const newClients = [...clients];

        let vacantSeatIdx = newClients.findIndex(c => c.Player.TablePos === netData.Client.Player.TablePos);
        if (vacantSeatIdx === -1)
          vacantSeatIdx = clients.findIndex(c => c._ID);

        if (vacantSeatIdx !== -1) {
          if (!newClients[vacantSeatIdx]._ID)
            console.error(`${pfx}: tried to set ${netData.Client.Name} to occupied seat: c[${vacantSeatIdx}]`);
          else {
            console.log(`${pfx}: setting c[${vacantSeatIdx}] => ${netData.Client.Name}`)
            newClients[vacantSeatIdx] = netData.Client;
          }
        } else {
          console.error(`${pfx}: couldnt find vacant seat idx ${vacantSeatIdx} [${clients.map(c => c.Name)}] nc.c ${nullClient.Name}`);
        }

        return newClients;
      });
      break;
    case NETDATA.PLAYER_RECONNECTING:
      updatePlayer({
        ...netData.Client,
        Player: {
          ...netData.Client.Player,
          isDisconnected: true,
        },
      });
      break;
    case NETDATA.PLAYER_RECONNECTED:
      updatePlayer({
        ...netData.Client,
        Player: {
          ...netData.Client.Player,
          isDisconnected: false,
        },
      });
      if (netData.Client.ID === yourClientRef.current?.ID)
        setModalOpen(false);

      break;
    case NETDATA.PLAYER_LEFT: {
      console.log(`player left: id: ${netData.Client.ID} n: ${netData.Client.Player.Name}`);
      updatePlayer(netData.Client, NewNullClient());

      // TODO: log more specific name info
      setChatMsgs(msgs => [...msgs, `<server-msg> ${netData.Client.Player.Name} left the table`]);
      setNumPlayers(netData.Table.NumPlayers);

      if (netData.Client.ID === yourClientRef.current.ID)
        setIsAdmin(false);

      break;
    }
    case NETDATA.MAKE_ADMIN:
      setIsAdmin(true);
      setGameOpts(opts => ({
        ...opts,
        websocketOpts: {
          ...opts.websocketOpts,
          Client: netData.Client,
        },
        roomSettings: netData.RoomSettings,
      }));
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
      setCurHand(netData.Msg);
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
      setCurHand(null);
      break;
    case NETDATA.ELIMINATED:
      if (netData.Client.ID === yourClientRef.current.ID) {
        setIsAdmin(false);
        setIsSpectator(true);
        setModalType('');
        setModalTxt(arr => [...arr, 'you have been eliminated']);
        setModalOpen(true);
      }
      // XXX: sometimes an UPDATE_PLAYER is being processed after
      //      PLAYER_LEFT, causing the players array to retain
      //      the eliminated player. if so, we will always remove them again
      //      from here for now.
      updatePlayer(netData.Client, NewNullClient());
      setChatMsgs(msgs => [...msgs, netData.Msg]);
      break;
    case NETDATA.FLOP:
    case NETDATA.TURN:
    case NETDATA.RIVER:
      setCommunity(netData.Table.Community);
      updateTable(netData);
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
  }, [applyYourClient, netData._noShallowCompare, yourClientRef, updatePlayer, updateRoom, updateTable]);
  /* eslint-enable react-hooks/set-state-in-effect */

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    normalizePlayersForSeats(numSeats);
  }, [normalizePlayersForSeats, numSeats]);
  /* eslint-enable react-hooks/set-state-in-effect */

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
    setGameOpts(opts => ({
      ...opts,
      isAdmin,
      creatorToken: isAdmin && gameOpts.creatorToken ? undefined : opts.creatorToken,
      creatorTokenRoomID: isAdmin && gameOpts.creatorToken ? undefined : opts.creatorTokenRoomID,
    }));
  }, [isAdmin, setGameOpts, gameOpts.creatorToken]);

  useEffect(() => {
    console.log(`isspec: ${isSpectator} ${yourClient}`);
    if (isSpectator && yourClientRef.current?.Player)
      socket?.send((new NetData(yourClientRef.current, NETDATA.PLAYER_LEFT)).toMsgPack());
  }, [isSpectator, yourClientRef, socket]);

  const dealerAndBlinds = {dealer, smallBlind, bigBlind};
  const isCompactRoom = innerWidth !== null && innerWidth <= 1920;

  const playerListPlayersProps = {
    players, curPlayer, playerHead, yourClient,
    isSpectator, keyPressed, socket, tableState,

    dealerAndBlinds,
  };

  const playerListSideProps = {
    players, curPlayer, curHand, playerHead,
    yourClient, keyPressed, tableState,

    dealerAndBlinds,

    innerTableItem: true,
  };

  return (
    <>
    <TableModal
      {...{modalType, modalTxt, setModalTxt, modalOpen, setModalOpen, setShowGame, setIsSpectator}}
      setFormData={setSettingsFormData}
    />
    <div
      className={cx(
        styles.tableGrid,
        dmMono.className,
        isCompactRoom && styles.compactTableGrid
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
            {...playerListPlayersProps}
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
          {...playerListPlayersProps}
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
            {...playerListSideProps}
            sideNum={2}
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
            {...playerListSideProps}
            sideNum={1}
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
            {...playerListSideProps}
            sideNum={3}
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
            {...playerListSideProps}
            sideNum={0}
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
          {...playerListPlayersProps}
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
            {...playerListPlayersProps}
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
            name: {
              yourClient?.Name
                ? yourClient.Name
                : <span style={{ fontStyle: 'italic' }}>noname</span>
            }
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
