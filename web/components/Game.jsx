import React, {useEffect, useState} from 'react';

import Image from 'next/image';
import Link from 'next/link';
import { Literata } from 'next/font/google';
import useSWRSubscription from 'swr/subscription';

import {CSSTransition } from 'react-transition-group';

import Tablenew from '@/components/Tablenew';

import styles from '@/styles/Game.module.css';

const literata = Literata({
  subsets: ['latin'],
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

export default function Game({ websocketOpts, setShowGame }) {
  const [socket, setSocket] = useState(null);

  const { data, error } = useSWRSubscription('ws://10.0.1.2:7755/web', (key, { next }) => {
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
            console.warn('msg', msg);
            
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

    return () => socket.close();
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
    
}
