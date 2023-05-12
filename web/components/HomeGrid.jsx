import Image from 'next/image';

import React, {useState, useEffect, useCallback, useRef} from 'react';

import { Exo } from 'next/font/google';

import styles from '@/styles/Home.module.css'; // XXX tmp until i move stuff

import NewGameForm from '@/components/NewGameForm';

const exo = Exo({ subsets: ['latin'] });

export default function HomeGrid({ newGameFormData, setNewGameFormData, isVisible, setShowGrid}) {
  const [showNewGameForm, setShowNewGameForm] = useState(false);

  const gridRef = useRef();
  const newGameRef = useRef();

  const toggleGrid = useCallback((isOn) => {
    let visibleCard;
    if (showNewGameForm) {
      //visibleCard = document.getElementById('newGame');
      visibleCard = newGameRef.current;
    }

    for (const el of gridRef.current.children) {
      if (el !== visibleCard) {
        if (isOn)
          el.classList.remove('hidden');
        else
          el.classList.add('hidden');
        el.classList.remove(styles.selectedCard);
        console.log(`classList now: ${el.classList}`);
      }
    }

    if (visibleCard) {
      setTimeout(() => {
        visibleCard.classList.add(styles.selectedCard);
      }, 500);
      console.log(`added selectedCard to ${visibleCard}`)
    }
  }, [showNewGameForm]);

  useEffect(() => {
    toggleGrid(!showNewGameForm);
    console.log(`showNewGameForm: ${showNewGameForm}`);
  }, [showNewGameForm]);

  useEffect(() => {
    if (newGameFormData) {
      setShowGrid(false);
    }
  }, [newGameFormData]);

  // unmount cleanup to ensure grid items are not open
  // when user returns to grid from deeper components
  useEffect(() => {
    return () => {
      setShowNewGameForm(false);
    };
  }, []);

  if (!isVisible)
    return;

  return <div className={styles.grid} ref={gridRef}>
    <a
      href="/a"
      className={styles.card}
      target="_blank"
      rel="noopener noreferrer"
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
      <p className={exo.className}>
        current games: 7
      </p>
    </a>

    <div
      ref={newGameRef}
      className={styles.card}
      onClick={() => {setShowNewGameForm(!showNewGameForm)}}
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
        setFormData={setNewGameFormData}
      />
    </div>

    <a
      href="/c"
      className={styles.card}
      target="_blank"
      rel="noopener noreferrer"
      tabIndex={0}
    >
      <h2 className={exo.className}>
        join room{' '}
        <span>
          <Image
            src={'/pokerchip3.png'}
            width={20}
            height={20}
            alt='chip'
          />
        </span>
      </h2>
      {/*<p className={exo.className}>
        ...
      </p>*/}
    </a>
  </div>;
}
