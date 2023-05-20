import React, { useContext, useEffect, useState, useRef, useCallback } from 'react';

import { useRouter } from 'next/router';
import Image from 'next/image';
import Link from 'next/link';
import { Literata } from 'next/font/google';

import useSWRSubscription from 'swr/subscription';

import {CSSTransition } from 'react-transition-group';

import { cloneDeep } from 'lodash';

import { GameContext } from '@/GameContext';
import NewGameForm from '@/components/NewGameForm';
import Tablenew from '@/components/Tablenew';

import { NETDATA, NetData } from '@/lib/libgopoker';

import homeStyles from '@/styles/Home.module.css';
import gameStyles from '@/styles/Game.module.css';

const literata = Literata({
  subsets: ['latin'],
  weight: '500',
});

import { decode } from '@msgpack/msgpack';

/*const Template = ({ children }) => {
  const logoImgRef = useRef(null);

  const toggleSpin = useCallback(() => {
    if (logoImgRef.current.classList.contains(homeStyles.pauseAnimation))
      logoImgRef.current.classList.remove(homeStyles.pauseAnimation);
    else
      logoImgRef.current.classList.add(homeStyles.pauseAnimation);
  }, [logoImgRef]);

  return (
    <>
      <Head>
        <title>gopoker - shirkadeh.org</title>
        <meta name="header" content="gopoker webclient" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" href="/favicon.ico" />
      </Head>
      <main className={homeStyles.main}>
        <div className={homeStyles.header}>
          <div className={`${homeStyles.logo} ${homeStyles.unselectable}`}>
            <h1>g</h1>
            <Image
              ref={logoImgRef}
              className={homeStyles.logoImgSpin}
              priority
              src={'/pokerchip3.png'}
              width={75}
              height={75}
              alt='o'
              onClick={toggleSpin}
            />
            <h1>poker</h1>
          </div>
          <p>current games: {'...'}</p>
        </div>
        <div className={homeStyles.center} id='center'>
          { children }
        </div>
      </main>
    </>
  );
}*/

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

  const { data, error } = useSWRSubscription(roomURL, (key, { next }) => {
    let socket;
    try {
      next(null); // need to reset error on Game remounts

      socket = new WebSocket(key);
      socket.addEventListener('open', (event) => {
        socket.send(websocketOpts.toMsgPack());
      });
      socket.addEventListener('message',
        async (event) => {
          try {
            //const msg = JSON.parse(event.data.toString());
            //const msg = await decodeFromBlob(event.data);
            const msg = decode(await event.data.arrayBuffer(), { useBigInt64: true });
            console.warn('Game: recv msg:', msg);

            msg.ShallowThis = Math.random();
            next(null, msg);
          } catch(e) {
            console.error(`msgpack decode(): err: ${e}`);
            next(e);
          }
      });
      socket.addEventListener('error', (event) => {
        console.error('websocket err', event.error);
        next(event.error ?? new Error('unspecified'));
      });
      setSocket(socket);
    } catch (e) {
      next(e);
    }

    return () => {
      socket.send(new NetData(null, NETDATA.CLIENT_EXITED).toMsgPack());
      socket.close(1000, 'web client exited');
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
        <p>{errMsg && `error: ${errMsg}` || 'room not found'}</p>
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

export default function Room() {
  const router = useRouter();
  const { roomID } = router.query;

  const {gameOpts, setGameOpts} = useContext(GameContext);

  const { roomURL, websocketOpts, setShowGame } = gameOpts;

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
          if (res.status != 404) {
            const body = await res.json();
            setCheckRoomErr(body.error ?? 'not specified');
          }
          setRoomNotFound(true);
        }
      } catch (e) {
        setRoomNotFound(false);
        setCheckRoomErr('/api/check fetch failed');
      }
    }

    roomID && checkRoom();
  }, [roomID]);

  if (websocketOpts && !roomURL)
    setGameOpts(gameOpts => {
      return {...gameOpts, roomURL: `ws://10.0.1.2:7755/room/${roomID}/web`}
    });

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