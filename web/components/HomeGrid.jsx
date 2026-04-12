import React, {useState, useEffect, useCallback, useRef } from 'react';

import Image from 'next/image';
import { Exo } from 'next/font/google';

import RoomList from '@/components/RoomList';
import NewGameForm from '@/components/NewGameForm';

import styles from '@/styles/Home.module.css'; // XXX tmp until i move stuff

const exo = Exo({ subsets: ['latin', 'latin-ext'] });

function HomeGrid({ isVisible }) {
  const [showRoomList, setShowRoomList] = useState(false);
  const [showNewGameForm, setShowNewGameForm] = useState(false);

  const gridRef = useRef();
  const roomListRef = useRef();
  const newGameRef = useRef();
  const selectedTimers = useRef(new Map());

  const toggleGrid = useCallback((isOn, visibleCard) => {
    const el = visibleCard.current;

    if (isOn) {
      el.classList.add(styles.chipHold);
      selectedTimers.current.set(visibleCard, setTimeout(() => {
        el.classList.add(styles.selectedCard);
      }, 500));
      console.log(`added selectedCard to ${el}`)
    } else {
      clearTimeout(selectedTimers.current.get(visibleCard));
      el.classList.remove(styles.selectedCard);
      el.classList.remove(styles.chipHold);
    }
  }, []);

  const toggleRoomList = useCallback(() => {
    setShowRoomList(showRoomList => !showRoomList);
  }, [setShowRoomList]);

  const toggleNewGameForm = useCallback(() => {
    setShowNewGameForm(showNewGameForm => !showNewGameForm);
  }, [setShowNewGameForm]);

  useEffect(() => {
    toggleGrid(showNewGameForm, newGameRef);
    console.log(`showNewGameForm: ${showNewGameForm}`);
  }, [showNewGameForm, toggleGrid]);

  useEffect(() => {
    toggleGrid(showRoomList, roomListRef);
    console.log(`showRoomList: ${showRoomList}`);
  }, [showRoomList, toggleGrid]);

  if (!isVisible)
    return;

  return <div className={styles.grid} ref={gridRef}>
    <div
      ref={roomListRef}
      className={styles.card}
      onClick={toggleRoomList}
      onKeyDown={(e) => {
        e.key === 'Enter' && e.target.click()
      }}
      tabIndex={0}
    >
      <h2 className={exo.className}>
        view public rooms{' '}
        <span>
          <Image
            src={'/pokerchip3.png'}
            width={20}
            height={20}
            alt='chip'
          />
        </span>
      </h2>
      <RoomList isVisible={showRoomList} />
    </div>

    <div
      ref={newGameRef}
      className={styles.card}
      onClick={toggleNewGameForm}
      onKeyDown={(e) => {
        e.key === 'Enter' && e.target.click()
      }}
      tabIndex={0}
    >
      <h2 className={exo.className}>
        new game{' '}
        <span>
          <Image
            src={'/pokerchip3.png'}
            width={20}
            height={20}
            alt='chip'
          />
        </span>
      </h2>
      <NewGameForm
        isVisible={showNewGameForm}
      />
    </div>
  </div>;
}

HomeGrid.displayName = 'HomeGrid';

export default React.memo(HomeGrid);
