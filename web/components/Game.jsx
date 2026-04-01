import React, {useContext, useEffect, useState} from 'react';

import {useRouter} from 'next/router';
import { Literata } from 'next/font/google';

import {GameContext} from '@/GameContext';
import UnsupportedDevice from './UnsupportedDevice';
import Spinner from '@/components/Spinner';

import config from '@/serverConfig';

const literata = Literata({
  subsets: ['latin', 'latin-ext'],
  weight: '500',
});

/*import { decode } from '@msgpack/msgpack';

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
}*/

const GamePostDimCheck = React.memo(({ isVisible, setShowGame }) => {
  const {gameOpts, setGameOpts} = useContext(GameContext);

  const [fetchCalled, setFetchCalled] = useState(false);
  const [error, setError] = useState('');

  const router = useRouter();

  useEffect(() => {
    // eslint-disable-next-line
    console.log('Game mounted isVisible:', isVisible, 'fetchCalled:', fetchCalled, 'gameOpts:', gameOpts);

    return () => {
      console.log('Game unmounted');
    }
  }, []);

  useEffect(() => {
    if (isVisible &&
        gameOpts.roomSettings && !fetchCalled) {
      const createNewRoom = async () => {
        const { RoomName, Lock, Password, NumSeats } = gameOpts.roomSettings;
        // eslint-disable-next-line
        console.log('before createNewRoom: gameOpts', gameOpts);
        try {
          const res = await fetch('/api/new', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
              roomName: RoomName || '',
              lock: Lock,
              password: Password || '',
              numSeats: NumSeats || 7,
            })
          });

          if (!res.ok)
            throw new Error('request failed');

          const data = await res.json();
          const { creatorToken } = data;
          const roomURL = `${config.gopokerServerWSURL}${data.URL}/web`;

          setFetchCalled(true);
          setGameOpts(gameOpts => ({
            ...gameOpts,
            roomURL,
            creatorToken,
            creatorTokenRoomID: data.roomName,
            setShowGame,
          }));
          window.roomURL = roomURL;

          router.push({
            pathname: '/room/[roomID]',
            query: { roomID: data.roomName },
          });
        } catch (err) {
          console.log(`couldn't POST to /api/new: ${err}`);
          setError(err);
        }
      };

      createNewRoom();
    }
  }, [isVisible, fetchCalled, gameOpts.roomSettings,
      router, setGameOpts, setShowGame]);

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

  return (
    <Spinner msg={'creating new room...'} />
  );
});

GamePostDimCheck.displayName = 'GamePostDimCheck';

function Game({ isVisible, isUnsupportedDevice, setShowGame }) {
  return (
    isUnsupportedDevice
      ? <UnsupportedDevice isVisible={isVisible} showHomeBtn={true} />
      : <GamePostDimCheck {...{isVisible, setShowGame}} />
  );
}

Game.displayName = 'Game';

export default React.memo(Game);
