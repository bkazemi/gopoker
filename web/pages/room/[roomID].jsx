import React, { useContext, useEffect, useState, useRef, useCallback } from 'react';

import { useRouter } from 'next/router';
import Image from 'next/image';
import Link from 'next/link';
import { Literata } from 'next/font/google';

import useSWRSubscription from 'swr/subscription';

import {CSSTransition } from 'react-transition-group';

import { cloneDeep } from 'lodash';
import { v4 as uuidv4 } from 'uuid';

import { GameContext } from '@/GameContext';
import UnsupportedDevice from '@/components/UnsupportedDevice';
import NewGameForm from '@/components/NewGameForm';
import Tablenew from '@/components/Tablenew';

import config from '@/serverConfig';

import { NETDATA, NetData } from '@/lib/libgopoker';

import homeStyles from '@/styles/Home.module.css';
import gameStyles from '@/styles/Game.module.css';

const literata = Literata({
  subsets: ['latin'],
  weight: '500',
});

import { decode } from '@msgpack/msgpack';

const Connect = () => {
  const {gameOpts, setGameOpts} = useContext(GameContext);
  const [socket, setSocket] = useState(null);

  const { roomURL, creatorToken, setShowGame } = gameOpts;
  let { websocketOpts } = gameOpts;

  if (creatorToken) {
    console.log(`Connect: setting password to creator token (${creatorToken})`);
    websocketOpts = cloneDeep(websocketOpts);
    websocketOpts.Client.Settings.Password = creatorToken;
  }

  useEffect(() => {
    console.log('Connect mounted');
    console.log('Connect: roomURL: ', roomURL);

    return () => {
      console.log('Connect unmounted');
    };
  }, []);

  // FIXME: when a player is eliminated, NetDataUpdatePlayer, NetDataPlayerLeft & NetDataEliminated
  // sometimes are being processed out of order, due to async nature of SWR
  const { data, error } = useSWRSubscription(roomURL, (key, { next }) => {
    let gameSocket;
    try {
      next(null); // need to reset error on Game remounts

      gameSocket = new WebSocket(key);
      gameSocket.addEventListener('open', (event) => {
        gameSocket.send(websocketOpts.toMsgPack());
      });
      gameSocket.addEventListener('message',
        async (event) => {
          try {
            //const msg = JSON.parse(event.data.toString());
            //const msg = await decodeFromBlob(event.data);
            const msg = decode(await event.data.arrayBuffer(), { useBigInt64: true });
            console.warn('Game: recv msg:', msg);

            msg._noShallowCompare = uuidv4();
            next(null, msg);
          } catch(e) {
            console.error(`msgpack decode(): err: ${e}`);
            next(e);
          }
      });
      gameSocket.addEventListener('error', (event) => {
        console.error('websocket err', event.error);
        next(event.error ?? new Error('unspecified'));
      });
      setSocket(gameSocket);
    } catch (e) {
      next(e);
    }

    return () => {
      gameSocket.send(new NetData(null, NETDATA.CLIENT_EXITED).toMsgPack());
      gameSocket.close(1000, 'web client exited');
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
     {...{socket, websocketOpts, setShowGame}}
     netData={data}
   />
  );
}

const CheckRoom = () => (
  <div className={gameStyles.spinner}>
    <p className={literata.className}>checking if room exists...</p>
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
            setCheckRoomErr(body.error ?? 'not specified');
          }
          setRoomNotFound(true);
        }
      } catch (e) {
        setRoomNotFound(true);
        setCheckRoomErr('/api/check fetch failed');
      }
    }

    if (gameOpts.creatorToken)
      setRoomNotFound(false);
    else
      roomID && checkRoom();
  }, [roomID]);

  useEffect(() => {
    if (!reset && websocketOpts && !roomURL)
      setGameOpts(gameOpts => ({
        ...gameOpts,
        roomURL: `${config.gopokerServerWSURL}/room/${roomID}/web`
      }));
  }, [websocketOpts, roomURL, reset]);

  if (gameOpts.goHome)
    return;

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
        {
          (roomNotFound === undefined && <CheckRoom />) ||
          (
            (roomNotFound && <RoomNotFound errMsg={checkRoomErr} router={router} />) ||
            (
              !roomURL &&
              <NewGameForm isVisible={true} isDirectLink={true} /> ||
              <Connect />
            )
          )
        }
      </CSSTransition>
  );
}

export default function Room() {
  //const {gameOpts, setGameOpts} = useContext(GameContext);

  const screenWidth = typeof window !== 'undefined' ? window.innerWidth : 0;
  const isUnsupportedDevice = screenWidth < 1080;

  //if (gameOpts.goHome)
  //  return;

  return (
    isUnsupportedDevice
      ? <UnsupportedDevice showHomeBtn={true} />
      : <RoomPostDimCheck />
  );
}
