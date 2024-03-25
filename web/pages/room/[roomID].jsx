import React, { useContext, useEffect, useState, useRef } from 'react';

import { useRouter } from 'next/router';
import Image from 'next/image';
import Link from 'next/link';
import { Literata } from 'next/font/google';
import dynamic from 'next/dynamic';

import useSWRSubscription from 'swr/subscription';

import {CSSTransition } from 'react-transition-group';

import { cloneDeep } from 'lodash';
import { v4 as uuidv4 } from 'uuid';

import { GameContext } from '@/GameContext';
import NewGameForm from '@/components/NewGameForm';
import Tablenew from '@/components/Tablenew';

const UnsupportedDevice = dynamic(() => import('@/components/UnsupportedDevice'), {
  ssr: false,
});

import config from '@/serverConfig';

import { NETDATA, NetData } from '@/lib/libgopoker';

import homeStyles from '@/styles/Home.module.css';
import gameStyles from '@/styles/Game.module.css';

const literata = Literata({
  subsets: ['latin'],
  weight: '500',
});

import { decode } from '@msgpack/msgpack';

const createWebSocket = (key, roomID, websocketOpts, setSocket, socketRef, setConnStatus, next, tryCnt) => {
  if (tryCnt > 2) {
    setConnStatus('closed');
    return;
  }

  const gameSocket = new WebSocket(key);

  gameSocket.addEventListener('message',
    async (event) => {
      try {
        //const msg = JSON.parse(event.data.toString());
        //const msg = await decodeFromBlob(event.data);
        const msg = decode(await event.data.arrayBuffer(), { useBigInt64: true });
        console.warn('Game: recv msg:', msg);

        msg._noShallowCompare = uuidv4();
        next(null, msg);
        if (tryCnt) // XXX
          tryCnt = 0;
      } catch(e) {
        console.error(`msgpack decode(): err: ${e}`);
        next(e);
      }
  });

  // we listen to error for debugging purposes, but all the error handling is
  // dealt with in the close handler
  gameSocket.addEventListener('error', (event) => {
    console.error('websocket err', event);
  });

  gameSocket.addEventListener('close', async (event) => {
    if (!event.wasClean || event.code !== 1000) {
      console.log('websocket had an unclean exit. attempting to reconnect...');
      console.log('making sure the room still exists...')
      const res = await fetch(`/api/check/${roomID}`);
      if (!res.ok) {
        const body = await res.text();
        next(
          res.status === 404
            ? new Error(`room "${roomID}" doesn't exist anymore`)
            : body?.error ?? `code ${res.code} reason unspecified`
        );

        return;
      }
      setConnStatus('rc');
      tryCnt++;
      await new Promise(res => setTimeout(res, 1 * 1000));
      createWebSocket(key, roomID, websocketOpts, setSocket, socketRef, setConnStatus, next, tryCnt);
      //createWebSocket(...arguments);
    } else {
      console.log('websocket had clean close', event);
    }
  });

  // set up open listener to make sure we don't miss the
  // first response msg in the message listener (unlikely but possible)
  gameSocket.addEventListener('open', (event) => {
    console.log('websocket open: trycnt', tryCnt);
    if (tryCnt > 0) {
      if (!window.privID) {
        console.error('createWebSocket: reconnect attempted with falsey window.privID');
        setConnStatus('closed');
        return;
      }
      console.log(`websocket open: reconnect attempt #${tryCnt}`);
      websocketOpts.Request = NETDATA.PLAYER_RECONNECTING;
      websocketOpts.Msg = window.privID;
    }
    gameSocket.send(websocketOpts.toMsgPack());
  });

  socketRef.current = gameSocket;
  setSocket(gameSocket);
  window.socket = gameSocket;
};

const Connect = ({ roomID }) => {
  const {gameOpts, setGameOpts} = useContext(GameContext);
  const [connStatus, setConnStatus] = useState('ok');
  const [socket, setSocket] = useState(null);
  const socketRef = useRef(null);
  const creatorTokenRef = useRef(null);

  const { roomURL, creatorToken, setShowGame } = gameOpts;
  let { websocketOpts } = gameOpts;

  if (creatorToken) {
    console.log(`Connect: setting password to creator token (${creatorToken})`);
    websocketOpts = cloneDeep(websocketOpts);
    websocketOpts.Client.Settings.Password = creatorToken;
    creatorTokenRef.current = creatorToken;
  }

  useEffect(() => {
    console.log('Connect mounted');
    // eslint-disable-next-line
    console.log('Connect: roomURL: ', roomURL);

    setGameOpts(opts => ({
      ...opts,
      isCompactRoom: window.innerWidth <= 1920,
    }));

    return () => {
      console.log('Connect unmounted');
      if (creatorTokenRef.current) { // token invalidated after first use
        console.log('Connect: removing creatorToken');
        setGameOpts(opts => ({
          ...opts,
          creatorToken: undefined,
          isCompactRoom: false,
        }));
      }
    };
  }, [setGameOpts]);

  // FIXME: when a player is eliminated, NetDataUpdatePlayer, NetDataPlayerLeft & NetDataEliminated
  // sometimes are being processed out of order, due to async nature of SWR
  const { data, error } = useSWRSubscription(roomURL, (key, { next }) => {
    try {
      next(null); // need to reset error on Game remounts

      createWebSocket(key, roomID, websocketOpts, setSocket, socketRef, setConnStatus, next, 0);
    } catch (e) {
      next(e);
    }

    return () => {
      if (socketRef.current?.readyState === WebSocket.OPEN) {
        socketRef.current.send(new NetData(null, NETDATA.CLIENT_EXITED).toMsgPack());
        socketRef.current.close(1000, 'web client exited');
      }
    }
  });

  if (error)
    return (
      <div
        className={literata.className}
        style={{
          display: 'flex',
          flexDirection: 'column',
          fontSize: '1.5rem',
          fontWeight: 'bold'
        }}
      >
        <p>failed to connect to server - error: { error.message }</p>
        <Link
          href={'/'}
          style={{
            alignSelf: 'center'
          }}
        >
          <button
            style={{
              width: '100px',
              padding: '5px',
              marginTop: '1rem',
            }}
          >
            go back
          </button>
        </Link>
      </div>
    );

  if (!data) return (
    <div className={gameStyles.spinner}>
      <p className={literata.className}>connecting to server...</p>
      <Image
        src='/pokerchip3.png'
        width={100} height={100}
        alt='spinner'
      />
    </div>
  );

  return (
   <Tablenew
     {...{socket, websocketOpts, connStatus, setShowGame}}
     netData={data}
   />
  );
}

const Spinner = ({ isCheckRoom }) => (
  <div className={gameStyles.spinner}>
    { isCheckRoom && <p className={literata.className}>checking if room exists...</p> }
    <Image
      src='/pokerchip3.png'
      width={100} height={100}
      alt='spinner'
    />
  </div>
);

const RoomNotFound = ({ errMsg, router }) => (
  <div
      className={literata.className}
      style={{
        display: 'flex',
        flexDirection: 'column',
        fontSize: '1.5rem',
        fontWeight: 'bold'
      }}
    >
      {
        <p
          style={{
            maxWidth: '33vw',
            maxHeight: '50vh',
            wordWrap: 'break-word',
          }}
        >
          {errMsg && `error: ${errMsg}` || 'room not found'}
        </p>
      }
      <button
        style={{
          width: '100px',
          padding: '5px',
          alignSelf: 'center',
          marginTop: '1rem',
        }}
        onClick={() => router.push('/')}
      >
        go back
      </button>
    </div>
);

function RoomPostDimCheck() {
  const router = useRouter();
  const { roomID } = router.query;

  const {gameOpts, setGameOpts} = useContext(GameContext);

  const { roomURL, websocketOpts, reset, setShowGame } = gameOpts;

  const [roomNotFound, setRoomNotFound] = useState(undefined);
  const [checkRoomErr, setCheckRoomErr] = useState('');

  // creatorToken is guaranteed to be set before it's consumed in the
  // checkRoom useEffect
  const creatorToken = gameOpts.creatorToken;

  useEffect(() => {
    const checkRoom = async () => {
      try {
        console.log('roomID:', roomID);
        const res = await fetch(`/api/check/${roomID}`);
        if (res.ok)
          setRoomNotFound(false);
        else {
          if (res.status === 403) {
            setRoomNotFound(true);
            setCheckRoomErr(`room "${roomID}" is locked.`);
          } else if (res.status != 404) {
            const body = await res.json();
            setCheckRoomErr(body.error ?? `code ${res.status} reason unspecified`);
          }
          setRoomNotFound(true);
        }
      } catch (e) {
        setRoomNotFound(true);
        setCheckRoomErr('/api/check fetch failed');
      }
    }

    if (creatorToken)
      setRoomNotFound(false);
    else
      roomID && checkRoom();
  }, [roomID]);

  useEffect(() => {
    if (!reset && websocketOpts && !roomURL) {
      const roomURL = `${config.gopokerServerWSURL}/room/${roomID}/web`;
      setGameOpts(gameOpts => ({
        ...gameOpts,
        roomURL,
      }));
      window.roomURL = roomURL;
    }
  }, [websocketOpts, roomURL, reset, setGameOpts]);

  if (gameOpts.goHome)
    return;

  const render = () => {
    if (roomNotFound === undefined)
      return <Spinner isCheckRoom={true} />;

    if (roomNotFound)
      return <RoomNotFound errMsg={checkRoomErr} router={router} />;

    if (!roomURL)
      return <NewGameForm isVisible={true} isDirectLink={true} />;

    return <Connect roomID={roomID} />;
  };

  return (
    <CSSTransition
      //in={showGame}
      in={true}
      timeout={500}
      classNames={{
        enter: homeStyles.fadeEnter,
        enterActive: homeStyles.fadeEnterActive,
        exit: homeStyles.fadeExit,
        exitActive: homeStyles.fadeExitActive,
      }}
      unmountOnExit
      onExited={() => {
        setShowGame(false);
        setShowGrid(true);
      }}
    >
      { render() }
    </CSSTransition>
  );
}

export default function Room() {
  //const {gameOpts, setGameOpts} = useContext(GameContext);
  const [isUnsupportedDevice, setIsUnsupportedDevice] = useState(false);

  const [isReadyForRender, setIsReadyForRender] = useState(false);

  useEffect(() => {
    const screenWidth = window?.innerWidth;
    setIsUnsupportedDevice(screenWidth < 1080);

    setIsReadyForRender(true);
  }, []);

  //if (gameOpts.goHome)
  //  return;

  if (!isReadyForRender)
    return <Spinner />

  return (
    isUnsupportedDevice
      ? <UnsupportedDevice isVisible={true} showHomeBtn={true} />
      : <RoomPostDimCheck />
  );
}
