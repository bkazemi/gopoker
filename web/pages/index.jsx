import Head from 'next/head'
import Image from 'next/image'
import { Exo } from 'next/font/google'
import styles from '@/styles/Home.module.css'

import React, { useEffect, useState } from 'react';

import { CSSTransition } from 'react-transition-group';

import HomeGrid from '@/components/HomeGrid';
import Game from '@/components/Game';

const exo = Exo({ subsets: ['latin'] });

export default function Home() {
  const [newGameFormData, setNewGameFormData] = useState(null);
  const [showGame, setShowGame] = useState(false);
  const [showGrid, setShowGrid] = useState(true);

  return (
    <>
      <Head>
        <title>gopoker - shirkadeh.org</title>
        <meta name="header" content="gopoker webclient" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" href="/favicon.ico" />
      </Head>
      <main className={styles.main}>
        <div className={styles.header}>
          <div className={`${styles.logo} ${styles.unselectable}`}>
            <h1>g</h1>
            <Image
              priority
              src={'/pokerchip3.png'}
              width={75}
              height={75}
              alt='o'>
            </Image>
            <h1>poker</h1>
          </div>
          <p>current games: {'...'}</p>
        </div>

        <div className={styles.center} id='center'>
          <CSSTransition
            in={showGame}
            timeout={500}
            classNames={{
              enter: styles.fadeEnter,
              enterActive: styles.fadeEnterActive,
              exit: styles.fadeExit,
              exitActive: styles.fadeExitActive,
            }}
            unmountOnExit
            onExited={() => {
              setShowGame(false);
              setShowGrid(true);
            }}
          >
            <Game
              websocketOpts={newGameFormData}
              setShowGame={setShowGame}
            />
          </CSSTransition>
          <CSSTransition
            in={showGrid}
            timeout={500}
            classNames={{
              enter: styles.fadeEnter,
              enterActive: styles.fadeEnterActive,
              exit: styles.fadeExit,
              exitActive: styles.fadeExitActive,
            }}
            onExited={() => {
              setShowGrid(false);
              setShowGame(true);
            }}
          >
            <HomeGrid
              {...{newGameFormData, setNewGameFormData, setShowGrid}}
              isVisible={showGrid}
            />
          </CSSTransition>
        </div>
      </main>
    </>
  )
}
