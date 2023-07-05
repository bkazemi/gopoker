import Image from 'next/image';

import React, {useState, useEffect, useCallback, useRef, useContext} from 'react';

import { Exo } from 'next/font/google';

import styles from '@/styles/Home.module.css'; // XXX tmp until i move stuff

import RoomList from '@/components/RoomList';
import NewGameForm from '@/components/NewGameForm';

const exo = Exo({ subsets: ['latin', 'latin-ext'] });

function HomeGrid({ isVisible }) {
  const [showRoomList, setShowRoomList] = useState(false);
  const [showNewGameForm, setShowNewGameForm] = useState(false);

  const gridRef = useRef();
  const roomListRef = useRef();
  const newGameRef = useRef();

  const toggleGrid = useCallback((isOn, visibleCard) => {
    visibleCard = visibleCard.current;

    /*for (const el of gridRef.current.children) {
      if (el !== visibleCard) {
        /*if (isOn)
          el.classList.remove('hidden');
        else
          el.classList.add('hidden');
        el.classList.remove(styles.selectedCard);
        console.log(`classList now: ${el.classList}`);
      }
    }*/

    if (isOn) {
      setTimeout(() => {
        visibleCard.classList.add(styles.selectedCard);
      }, 500);
      console.log(`added selectedCard to ${visibleCard}`)
    } else {
      visibleCard.classList.remove(styles.selectedCard);
    }
  }, [showNewGameForm, showRoomList]);

  useEffect(() => {
    toggleGrid(showNewGameForm, newGameRef);
    console.log(`showNewGameForm: ${showNewGameForm}`);
  }, [showNewGameForm]);

  useEffect(() => {
    toggleGrid(showRoomList, roomListRef);
    console.log(`showRoomList: ${showRoomList}`);
  }, [showRoomList])

  /*useEffect(() => {
    console.log('HomeGrid: gameOpts.websocketOpts useEffect');

    if (gameOpts.websocketOpts)
      setShowGrid(false);
  }, [gameOpts.showGame]);*/

  // unmount cleanup to ensure grid items are not open
  // when user returns to grid from deeper components
  /*useEffect(() => {
    return () => {
      setShowNewGameForm(false);
    };
  }, []);*/

  if (!isVisible)
    return;

  return <div className={styles.grid} ref={gridRef}>
    <div
      ref={roomListRef}
      className={styles.card}
      onClick={() => setShowRoomList(!showRoomList)}
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
      onClick={() => setShowNewGameForm(!showNewGameForm)}
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

export default React.memo(HomeGrid);
