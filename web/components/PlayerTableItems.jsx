import React, { useState} from 'react';

import Image from 'next/image';

import styles from '@/styles/PlayerTableItems.module.css';

import { TABLE_STATE, cardToImagePath, PLAYERSTATE } from '@/lib/libgopoker';

const Cards = React.memo(({ client, isYourPlayer, side, tableState }) => {
  // we want the cards to be displayed at the same time,
  // so we make sure both card images are loaded before displaying them
  const [numCardsLoaded, setNumCardsLoaded] = useState(0);

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
    return <div
             className={styles.playerCards}
             style={{ opacity: (numCardsLoaded === 2) ? 1 : 0 }}
           >
            <Image
              src={'/cards/cardBack_blue5.png'}
              height={90}
              width={65}
              alt={'[card]'}
              onLoad={() => setNumCardsLoaded(numCards => numCards % 2 + 1)}
            />
            <Image
              src={'/cards/cardBack_blue5.png'}
              height={90}
              width={65}
              alt={'[card]'}
              style={style}
              onLoad={() => setNumCardsLoaded(numCards => numCards % 2 + 1)}
            />
      </div>
  else
    return <div
             className={styles.playerCards}
             style={{ opacity: (numCardsLoaded === client?.Player?.Hole?.Cards?.length) ? 1 : 0 }}
           >
      {
        client?.Player?.Hole?.Cards
          .map((c, idx) => {
            return <Image
              key={idx}
              src={cardToImagePath(c)}
              height={90}
              width={65}
              alt={`[${c.Name}]`}
              onLoad={() => setNumCardsLoaded(numCards =>
                numCards % client?.Player?.Hole?.Cards?.length + 1
              )}
            />;
        }) || null
      }
    </div>
});

Cards.displayName = 'Cards';

function PlayerTableItems({
  client, isYourPlayer, side,
  gridRow, gridCol, tableState
}) {
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
