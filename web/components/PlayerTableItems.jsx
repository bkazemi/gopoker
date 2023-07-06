import React, {useEffect, useState} from 'react';

import Image from 'next/image';

import styles from '@/styles/PlayerTableItems.module.css';

import { TABLE_STATE, cardToImagePath, PLAYERSTATE } from '@/lib/libgopoker';

const Cards = React.memo(({ client, isYourPlayer, side, tableState }) => {
  if (
    tableState === TABLE_STATE.NOT_STARTED ||
    client.Player.Action.Action === PLAYERSTATE.FOLD ||
    client.Player.Action.Action === PLAYERSTATE.MIDROUND_ADDITION
  ) {
    return;
  }

  let style = {};
  if (side === 'left' || side === 'right')
    style = {
      'position': 'relative',
      'right': '32.5px',
    }

  if (!isYourPlayer && !client?.Player?.Hole?.Cards)
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
          .map((c, idx) => {
            return <Image
              key={idx}
              src={cardToImagePath(c)}
              height={90}
              width={65}
              alt={`[${c.Name}]`}
            />;
        }) || null
      }
    </div>
});

Cards.displayName = 'Cards';

const Positions = React.memo(({ tableState, isDealer, isSmallBlind, isBigBlind }) => {
  if (tableState === TABLE_STATE.NOT_STARTED ||
     (!isDealer && !isBigBlind && !isSmallBlind))
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
});

Positions.displayName = 'Positions';

/*const seatImgMap = new Map([
  ['dealer', '/D.png'],
  ['smallBlind', '/SB.png'],
  ['bigBlind', '/BB.png']
]);*/

function PlayerTableItems({
  client, isYourPlayer, dealerAndBlinds,
  side, gridRow, gridCol, tableState
}) {
  // currently disabled
  /*const [isDealer, setIsDealer] = useState(false);
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
  }, [dealerAndBlinds, client]);*/

  if (client._ID === 'vacant')
    return;

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
      {/*<Positions {...{tableState, isDealer, isSmallBlind, isBigBlind}} />*/}
      <Cards
        client={client}
        isYourPlayer={isYourPlayer}
        side={side}
        tableState={tableState}
      />
    </div>
  );
}

PlayerTableItems.displayName = 'PlayerTableItems';

export default React.memo(PlayerTableItems);
