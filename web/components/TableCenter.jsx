import React from 'react';

import Image from 'next/image';
import { Literata } from 'next/font/google';

import {TABLE_STATE, NETDATA, NetData, cardToImagePath } from '@/lib/libgopoker';

import styles from '@/styles/TableCenter.module.css';

const literata = Literata({ subsets: ['latin'], weight: '500' });

export default function TableCenter({ isAdmin, tableState, community, yourClient, socket }) {
  return (
    <div>
      {
        community.length &&
        <div className={styles.community}>
          {
            community
              .map((c, idx) => {
                return <Image
                  key={idx}
                  src={cardToImagePath(c)}
                  height={100}
                  width={66.66666667}
                  alt={c.Name}
                />
              })
          }
        </div> || ''
      }
      {
        (isAdmin && tableState === TABLE_STATE.NOT_STARTED) &&
          <div className={styles.preGame}>
            <button
              className={literata.className}
              onClick={() => {
                socket.send((new NetData(yourClient, NETDATA.START_GAME)).toMsgPack());
              }}
            >
              start game
            </button>
          </div>
      }
    </div>
  );
}
