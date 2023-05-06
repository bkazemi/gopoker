import React, {useEffect, useState} from 'react';

import Image from 'next/image';

import styles from '@/styles/PlayerTableItems.module.css';

import { TABLE_STATE, cardToImagePath } from '@/lib/libgopoker';

const Cards = ({ client, isYourPlayer, side, tableState }) => {
  if (tableState === TABLE_STATE.NOT_STARTED)
    return;

  let style = {};
  if (side === 'left' || side === 'right')
    style = {
      'position': 'relative',
      'right': '32.5px',
    }

  if (!isYourPlayer && tableState !== TABLE_STATE.SHOW_HANDS)
    return <div className={styles.playerCards}>
            <Image 
              src={'/cards/cardBack_blue5.png'}
              height={90}
              width={65}
              alt={'[card]'}
            />
            <Image 
              src={'/cards/cardBack_blue5.png'}
              height={90}
              width={65}
              alt={'[card]'}
              style={style}
            />
      </div>
  else
    return <div className={styles.playerCards}>
      {
        client?.Player?.Hole?.Cards
          .map(c => {
            return <Image
              src={cardToImagePath(c)}
              height={90}
              width={65}
              alt={`[${c.Name}]`}
            />;
        }) || ''
      }
    </div>
};

const Positions = ({ isDealer, isSmallBlind, isBigBlind }) => {
  if (!isDealer && !isBigBlind && !isSmallBlind)
    return;

  return (
    <div className={styles.positions}>
      {
        isDealer && 
        <Image
          src={'/D.png'}
          width={35}
          height={35}
          alt='[D]'
        />
      }
      {
        isSmallBlind && 
        <Image
          src={'/SB.png'}
          width={35}
          height={35}
          alt='[Sb]'
        />
      }
      {
        isBigBlind && 
        <Image
          src={'/BB.png'}
          width={35}
          height={35}
          alt='[Bb]'
        />
      }
    </div>
  );
};

/*const seatImgMap = new Map([
  ['dealer', '/D.png'],
  ['smallBlind', '/SB.png'],
  ['bigBlind', '/BB.png']
]);*/

export default function PlayerTableItems({ client, isYourPlayer, dealerAndBlinds,
  side, gridRow, gridCol, tableState }) {
  if (client._ID === 'vacant')
    return;

  const [isDealer, setIsDealer] = useState(false);
  const [isSmallBlind, setIsSmallBlind] = useState(false);
  const [isBigBlind, setIsBigBlind] = useState(false);

  const posSetStateMap = {
    dealer: setIsDealer,
    smallBlind: setIsSmallBlind,
    bigBlind: setIsBigBlind,
  };

  useEffect(() => {
    [...Object.entries(dealerAndBlinds)]
      .filter(([name, seat]) => {
        console.log(`dSB ${name} name: ${seat?.Player?.Name}  cname ${client?.Player?.Name}`);
        if (seat?.Player?.Name === client?.Player?.Name)
          return true;
        else
          posSetStateMap[name](false);
      })
      .map(([name]) => {
        console.log(`MAP ${client.Player.Name} ${name}`);
        posSetStateMap[name](true);
      });
  }, [dealerAndBlinds, client]);

  /*useEffect(() => {
  //  console.log(`isDSB: ${client?.Player?.Name} ${isDealer} ${isBigBlind} ${isSmallBlind}`);
  //}, [isDealer, isSmallBlind, isBigBlind]);
  
  useEffect(() => {
    console.log(`${client?.Player?.Name} D => ${isDealer}`);
  }, [isDealer]);
  useEffect(() => {
    console.log(`${client?.Player?.Name} Sb => ${isDealer}`);
  }, [isSmallBlind]);
  useEffect(() => {
    console.log(`${client?.Player?.Name} Bb => ${isBigBlind}`);
  }, [isBigBlind]);*/

  let justifyContent;
  if (side === 'top') {
    justifyContent = 'flex-start';
  } else if (side === 'bottom') {
    justifyContent = 'flex-end';
  }

  return (
    <div 
      className={styles.playerItems}
      style={{ justifyContent, gridRow: gridRow, gridColumn: gridCol }}
    >
      <Positions {...{isDealer, isSmallBlind, isBigBlind}} />
      <Cards
        client={client}
        isYourPlayer={isYourPlayer}
        side={side}
        tableState={tableState}
      />
    </div>
  );
}
