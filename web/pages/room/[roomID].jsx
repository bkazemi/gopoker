import React, { useContext, useEffect, useState, useRef } from 'react';

import { useRouter } from 'next/router';
import Link from 'next/link';
import dynamic from 'next/dynamic';
import { Literata } from 'next/font/google';

import useSWRSubscription from 'swr/subscription';

import {CSSTransition } from 'react-transition-group';

import { cloneDeep } from 'lodash';
import { v4 as uuidv4 } from 'uuid';
import { decode } from '@msgpack/msgpack';

import { GameContext } from '@/GameContext';
import NewGameForm from '@/components/NewGameForm';
import Tablenew from '@/components/Tablenew';
import Spinner from '@/components/Spinner';

const UnsupportedDevice = dynamic(() => import('@/components/UnsupportedDevice'), {
  ssr: false,
});

import useDeferredLoading from '@/lib/useDeferredLoading';
import useInitialWindowMetrics from '@/lib/useInitialWindowMetrics';
import config from '@/serverConfig';

import { NETDATA, NetData } from '@/lib/libgopoker';

import homeStyles from '@/styles/Home.module.css';

const literata = Literata({
  subsets: ['latin'],
  weight: '500',
});

const closeWebSocket = (socket) => {
  if (socket?.readyState === WebSocket.OPEN) {
    socket.send(new NetData(null, NETDATA.CLIENT_EXITED).toMsgPack());
    socket.close(1000, 'web client exited');
  } else if (socket?.readyState === WebSocket.CONNECTING) {
    socket.close(1000, 'web client exited');
  }
};

const createWebSocket = ({
  roomIDRef,
  websocketOpts,
  setSocket,
  socketRef,
  setConnStatus,
  next,
  tryCnt,
  signal,
  onFirstMessage,
}) => {
  if (signal.aborted)
    return;

  if (tryCnt > 2) {
    setConnStatus('closed');
    return;
  }

  const wsURL = `${config.gopokerServerWSURL}/room/${encodeURIComponent(roomIDRef.current)}/web`;
  const gameSocket = new WebSocket(wsURL);

  let joinConfirmed = false;
  gameSocket.addEventListener('message',
    async (event) => {
      try {
        //const msg = JSON.parse(event.data.toString());
        //const msg = await decodeFromBlob(event.data);
        const msg = decode(await event.data.arrayBuffer(), { useBigInt64: true });
        console.warn('Game: recv msg:', msg);

        if (!joinConfirmed) {
          joinConfirmed = true;
          onFirstMessage?.();
        }

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
    if (signal.aborted) {
      console.log('websocket close was intentional; skipping reconnect');
      return;
    }

    if (!event.wasClean || event.code !== 1000) {
      console.log('websocket had an unclean exit. attempting to reconnect...');
      console.log('making sure the room still exists...')
      const currentRoomID = roomIDRef.current;
      try {
        const res = await fetch(`/api/check/${encodeURIComponent(currentRoomID)}`, { signal });

        if (!res.ok) {
          const body = await res.text();

          let reason = body || `code ${res.status} reason unspecified`;
          try { reason = JSON.parse(body).error ?? reason; } catch {}

          next(
            res.status === 404
              ? new Error(`room "${currentRoomID}" doesn't exist anymore`)
              : new Error(reason)
          );

          return;
        }
      } catch (e) {
        if (signal.aborted)
          return;
        next(e);
        return;
      }
      setConnStatus('rc');
      await new Promise(res => setTimeout(res, 1 * 1000));
      if (signal.aborted)
        return;

      createWebSocket({
        roomIDRef,
        websocketOpts,
        setSocket,
        socketRef,
        setConnStatus,
        next,
        tryCnt: tryCnt + 1,
        signal,
        onFirstMessage,
      });
    } else {
      console.log('websocket had clean close', event);
    }
  });

  // set up open listener to make sure we don't miss the
  // first response msg in the message listener (unlikely but possible)
  gameSocket.addEventListener('open', (event) => {
    console.log('websocket open: trycnt', tryCnt);

    const socketOpts = tryCnt > 0 ? cloneDeep(websocketOpts) : websocketOpts;
    if (tryCnt > 0) {
      if (window.privID) {
        console.log(`websocket open: reconnect attempt #${tryCnt}`);
        socketOpts.Request = NETDATA.PLAYER_RECONNECTING;
        socketOpts.Msg = window.privID;
      } else {
        console.log('websocket open: no privID yet, retrying fresh join');
      }
    }

    setConnStatus('ok');
    gameSocket.send(socketOpts.toMsgPack());
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
  const roomIDRef = useRef(roomID);

  const { roomURL, creatorToken, creatorTokenRoomID, setShowGame } = gameOpts;
  let { websocketOpts } = gameOpts;
  const hasCreatorTokenForRoom = creatorToken && creatorTokenRoomID === roomID;

  if (hasCreatorTokenForRoom) {
    console.log(`Connect: setting password to creator token (${creatorToken})`);
    websocketOpts = cloneDeep(websocketOpts);
    websocketOpts.Client.Settings.Password = creatorToken;
  }

  useEffect(() => {
    roomIDRef.current = roomID;
  }, [roomID]);

  useEffect(() => {
    console.log('Connect mounted');
    // eslint-disable-next-line
    console.log('Connect: roomURL: ', roomURL);

    return () => {
      console.log('Connect unmounted');
    };
  }, []);

  // FIXME: when a player is eliminated, NetDataUpdatePlayer, NetDataPlayerLeft & NetDataEliminated
  // sometimes are being processed out of order, due to async nature of SWR
  const { data, error } = useSWRSubscription(roomURL, (_key, { next }) => {
    // Defer socket creation so React StrictMode's throwaway mount in development
    // can clean up before the creator token is used.
    const controller = new AbortController();
    const { signal } = controller;
    const clearCreatorToken = hasCreatorTokenForRoom
      ? () => {
          console.log('Connect: clearing creator token from gameOpts');
          setGameOpts(opts => ({
            ...opts,
            creatorToken: undefined,
            creatorTokenRoomID: undefined,
          }));
        }
      : undefined;
    next(null); // need to reset error on Game remounts
    const connectTimer = window.setTimeout(() => {
      try {
        createWebSocket({
          roomIDRef,
          websocketOpts,
          setSocket,
          socketRef,
          setConnStatus,
          next,
          tryCnt: 0,
          signal,
          onFirstMessage: clearCreatorToken,
        });
      } catch (e) {
        next(e);
      }
    }, 0);

    return () => {
      controller.abort();
      window.clearTimeout(connectTimer);
      closeWebSocket(socketRef.current);
      socketRef.current = null;
    }
  });

  const showConnectingSpinner = useDeferredLoading(!data && !error);

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
        <p>failed to connect to server: { error.message }</p>
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

  if (!data)
    return showConnectingSpinner ? <Spinner msg={'connecting to server...'} /> : null;

  return (
   <Tablenew
     {...{socket, websocketOpts, connStatus, setShowGame, roomIDRef}}
     netData={data}
   />
  );
}

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

const TransitionPlaceholder = () => (
  <div />
);

function RoomPostDimCheck() {
  const router = useRouter();
  const { roomID } = router.query;

  const {gameOpts, setGameOpts} = useContext(GameContext);

  const { roomURL, websocketOpts, setShowGame } = gameOpts;

  const [roomNotFound, setRoomNotFound] = useState(undefined);
  const [checkRoomErr, setCheckRoomErr] = useState('');
  const showCheckingSpinner = useDeferredLoading(roomNotFound === undefined);

  const prevRoomIDRef = useRef(roomID);
  const skipCheckRef = useRef(false);
  useEffect(() => {
    if (prevRoomIDRef.current !== roomID) {
      prevRoomIDRef.current = roomID;
      if (!gameOpts.roomRenamed) {
        setGameOpts(opts => ({ ...opts, roomURL: undefined, websocketOpts: undefined }));
        setRoomNotFound(undefined);
        setCheckRoomErr('');
      } else {
        skipCheckRef.current = true;
        setGameOpts(opts => ({ ...opts, roomRenamed: undefined }));
      }
    }
  }, [roomID, gameOpts.roomRenamed, setGameOpts]);

  useEffect(() => {
    if (skipCheckRef.current) {
      skipCheckRef.current = false;
      return;
    }

    if (gameOpts.creatorToken && gameOpts.creatorTokenRoomID === roomID) {
      setRoomNotFound(false);
      return;
    }

    const controller = new AbortController();

    const checkRoom = async () => {
      try {
        console.log('roomID:', roomID);
        const res = await fetch(`/api/check/${encodeURIComponent(roomID)}`, {
          signal: controller.signal,
        });
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
        if (controller.signal.aborted)
          return;
        setRoomNotFound(true);
        setCheckRoomErr('/api/check fetch failed');
      }
    }

    roomID && checkRoom();

    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roomID]);

  useEffect(() => {
    if (websocketOpts && !roomURL) {
      const roomURL = `${config.gopokerServerWSURL}/room/${encodeURIComponent(roomID)}/web`;
      setGameOpts(gameOpts => ({
        ...gameOpts,
        roomURL,
      }));
      window.roomURL = roomURL;
    }
  }, [websocketOpts, roomURL, setGameOpts]);

  if (gameOpts.goHome)
    return <TransitionPlaceholder />;

  const render = () => {
    if (roomNotFound === undefined)
      return showCheckingSpinner ? <Spinner msg={'checking if room exists...'} /> : <TransitionPlaceholder />;

    if (roomNotFound)
      return <RoomNotFound errMsg={checkRoomErr} router={router} />;

    if (!roomURL)
      return <NewGameForm isVisible={true} isDirectLink={true} />;

    if (!roomID)
      return <TransitionPlaceholder />;

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
  const { innerWidth } = useInitialWindowMetrics();

  if (innerWidth === null)
    return <Spinner />

  const isUnsupportedDevice = innerWidth < 1080;

  return (
    isUnsupportedDevice
      ? <UnsupportedDevice isVisible={true} showHomeBtn={true} />
      : <RoomPostDimCheck />
  );
}
