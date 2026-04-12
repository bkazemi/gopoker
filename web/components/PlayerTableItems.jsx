import React, { useState} from 'react';

import Image from 'next/image';
import cx from 'classnames';

import { TABLE_STATE, cardToImagePath, PLAYERSTATE } from '@/lib/libgopoker';

import styles from '@/styles/PlayerTableItems.module.css';

const Cards = React.memo(({ client, isYourPlayer, side, tableState }) => {
  // we want the cards to be displayed at the same time,
  // so we make sure both card images are loaded before displaying them
  const [numCardsLoaded, setNumCardsLoaded] = useState(0);

  const isLR = side === 'left' || side === 'right';
  const cardsClass = cx(
    styles.playerCards,
    isLR && styles.playerCardsLR,
  );
  const hiddenCardsClass = cx(
    styles.hiddenCards,
    side === 'top' && styles.hiddenCardsTop,
    side === 'left' && styles.hiddenCardsLeft,
    side === 'right' && styles.hiddenCardsRight,
  );
  const holeLen = client?.Player?.Hole?.Cards?.length || 0;

  if (
    tableState === TABLE_STATE.NOT_STARTED ||
    client.Player.Action.Action === PLAYERSTATE.FOLD ||
    client.Player.Action.Action === PLAYERSTATE.MIDROUND_ADDITION
  ) {
    return;
  }

  if (!isYourPlayer && !holeLen)
    return <div
             className={hiddenCardsClass}
             style={{ opacity: numCardsLoaded === 2 ? 1 : 0 }}
           >
            <div className={styles.hiddenCardsInner}>
              <div className={cx(styles.cardSlot, styles.cardSlotPrimary)}>
                <Image
                  src={'/cards/cardBack_blue5.png'}
                  height={90}
                  width={65}
                  alt={'[card]'}
                  onLoad={() => setNumCardsLoaded(numCards => numCards % 2 + 1)}
                />
              </div>
              <div className={cx(styles.cardSlot, styles.cardSlotSecondary)}>
                <Image
                  src={'/cards/cardBack_blue5.png'}
                  height={90}
                  width={65}
                  alt={'[card]'}
                  onLoad={() => setNumCardsLoaded(numCards => numCards % 2 + 1)}
                />
              </div>
            </div>
      </div>
  else
    return <div
             className={cardsClass}
             style={{ opacity: numCardsLoaded === holeLen ? 1 : 0 }}
           >
      {
        client?.Player?.Hole?.Cards
          .map((c, idx) => {
            return <div
              key={idx}
              className={cx(
                styles.cardSlot,
                idx === 0 ? styles.cardSlotPrimary : styles.cardSlotSecondary,
              )}
            >
              <Image
                src={cardToImagePath(c)}
                height={90}
                width={65}
                alt={`[${c.Name}]`}
                onLoad={() => setNumCardsLoaded(numCards =>
                  numCards % holeLen + 1
                )}
              />
            </div>;
        }) || null
      }
    </div>
});

Cards.displayName = 'Cards';

function PlayerTableItems({
  client, isYourPlayer, curHand, side,
  gridRow, gridCol, tableState
}) {
  if (client._ID)
    return;

  return (
    <div
      className={styles.playerItems}
      style={{ gridRow: gridRow, gridColumn: gridCol }}
    >
      {
        isYourPlayer && curHand &&
        <p className={styles.curHand}>
          { curHand }
        </p>
      }
      <Cards
        {...{client, isYourPlayer, side, tableState}}
      />
    </div>
  );
}

PlayerTableItems.displayName = 'PlayerTableItems';

export default React.memo(PlayerTableItems);
