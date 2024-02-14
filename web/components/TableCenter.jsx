import React, {useState} from 'react';

import Image from 'next/image';
import { Literata, DM_Mono } from 'next/font/google';

import cx from 'classnames';

import {TABLE_STATE, NETDATA, NetData, cardToImagePath } from '@/lib/libgopoker';

import styles from '@/styles/TableCenter.module.css';

const literata = Literata({ subsets: ['latin'], weight: '500' });
const dmMono = DM_Mono({ subsets: ['latin', 'latin-ext'], weight: '500' });

function TableCenter({ isAdmin, tableState, community, mainPot, yourClient, socket }) {
  const [numCardsLoaded, setNumCardsLoaded] = useState(0);

  return (
    <div>
      {
        community.length &&
        <div
          className={styles.community}
          style={{
            opacity: tableState === TABLE_STATE.FLOP ?
                                      (numCardsLoaded === 3 ? 1 : 0)
                                      : 1
          }}
        >
          {
            community
              .map((c, idx) => {
                return <Image
                  key={idx}
                  src={cardToImagePath(c)}
                  height={100}
                  width={66.66666667}
                  alt={c.Name}
                  onLoad={() =>
                    tableState === TABLE_STATE.FLOP && setNumCardsLoaded(numCards => numCards % 3 + 1)
                  }
                />
              })
          }
        </div> || null
      }
      {
        (!isAdmin && tableState === TABLE_STATE.NOT_STARTED) &&
        <p
          className={literata.className}
          style={{ fontSize: '1.3rem' }}
        >
          waiting for table admin to start the game
        </p>
      }
      {
        tableState !== TABLE_STATE.NOT_STARTED &&
        <div className={styles.mainPotContainer} >
          <Image
            src={'/mainPotChips.png'}
            height={50}
            width={50}
            alt={'mainPotChips logo'}
          />
          <p className={cx(styles.mainPot, dmMono.className)}>mainpot: { mainPot.Total.toLocaleString() }</p>
        </div>
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

TableCenter.displayName = 'TableCenter';

export default React.memo(TableCenter);
