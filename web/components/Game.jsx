import React, {useContext, useEffect, useState} from 'react';

import Image from 'next/image';
//import Link from 'next/link';
import {useRouter} from 'next/router';

import { Literata } from 'next/font/google';
//import useSWRSubscription from 'swr/subscription';

//import {CSSTransition } from 'react-transition-group';

import {GameContext} from '@/GameContext';
//import Tablenew from '@/components/Tablenew';

import config from '@/serverConfig';

//import { NETDATA, NetData } from '@/lib/libgopoker';

import styles from '@/styles/Game.module.css';

const literata = Literata({
  subsets: ['latin', 'latin-ext'],
  weight: '500',
});

import { decode } from '@msgpack/msgpack';

async function decodeAsync(stream) {
  const chunks = [];
  for await (const chunk of stream) {
    chunks.push(chunk);
  }
  console.warn(`chunks: ${new Uint8Array(chunks.flat()).buffer}`);
  const buffer = new Uint8Array(chunks.flat()).buffer;
  return decode(buffer, { useBigInt64: true });
}

async function decodeFromBlob(blob) {
  if (blob.stream) {
    return await decodeAsync(blob.stream());
  } else {
    return decode(await blob.arrayBuffer(), { useBigInt64: true });
  }
}

export default function Game({ isVisible, setShowGame }) {
  const {gameOpts, setGameOpts} = useContext(GameContext);

  const [fetchCalled, setFetchCalled] = useState(false);
  const [roomURL, setRoomUrl] = useState('');
  const [error, setError] = useState('');

  const router = useRouter();

  useEffect(() => {
    console.log('Game mounted isVisible:', isVisible, 'fetchCalled:', fetchCalled);

    return () => {
      console.log('Game unmounted');
    }
  }, []);

  useEffect(() => {
    if (isVisible && !fetchCalled) {
      const createNewRoom = async () => {
        const { RoomName, Lock, Password } = gameOpts.websocketOpts.Client.Settings.Admin;
        console.log('before createNewRoom: gameOpts.websocketOpts:', gameOpts.websocketOpts);
        try {
          const res = await fetch('/api/new', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
              roomName: RoomName || '',
              lock: Lock,
              password: Password || '',
              numSeats: 7,
            })
          });

          if (!res.ok)
            throw new Error('request failed');

          const data = await res.json();
          const { creatorToken } = data;
          const roomURL = `${config.gopokerServerWSURL}${data.URL}/web`;

          setFetchCalled(true);
          setGameOpts(gameOpts => {
            return {
              ...gameOpts,
              roomURL, creatorToken, setShowGame
            };
          });

          router.push(data.URL);
        } catch (err) {
          console.log(`couldn't POST to /api/new: ${err}`);
          setError(err);
        }
      };

      createNewRoom();
    }
  }, [isVisible, fetchCalled]);

  if (!isVisible)
    return;

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
        <p>failed to create new room - error: { error.message }</p>
        <button style={{
            width: '100px',
            alignSelf: 'center',
            padding: '5px',
            marginTop: '1rem',
          }}
          onClick={() => setShowGame(false)}
        >
          go back
        </button>
      </div>
    );

  if (roomURL === '')
    return (
      <div className={styles.spinner}>
        <p className={literata.className}>creating new room...</p>
        <Image
          src='/pokerchip3.png'
          width={100} height={100}
          alt='spinner'
        />
      </div>
    );
}

/*function GameTmp({ roomURL, websocketOpts, setShowGame }) {
  const [socket, setSocket] = useState(null);

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
        <button style={{
            width: '100px',
            alignSelf: 'center',
            padding: '5px',
            marginTop: '1rem',
          }}
          onClick={() => setShowGame(false)}
        >
          go back
        </button>
      </div>
    );

  if (!data) return (
    <div className={styles.spinner}>
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

}*/
