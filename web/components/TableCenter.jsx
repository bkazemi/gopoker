import React from 'react';

import Image from 'next/image';

import {TABLE_STATE, NETDATA, NetData, cardToImagePath } from '@/lib/libgopoker';

import styles from '@/styles/TableCenter.module.css';

export default function TableCenter({ isAdmin, tableState, community, yourClient, socket }) {
  return (
    <div>
      {
        community.length &&
        <div className={styles.community}>
          {
            community
              .map((c) => {
                return <Image
                  src={cardToImagePath(c)}
                  height={100}
                  width={66.6666667}
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
