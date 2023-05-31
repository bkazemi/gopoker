import { Exo } from 'next/font/google'
import styles from '@/styles/Home.module.css'

import React, { useCallback, useContext, useEffect, useState, useRef } from 'react';

import { CSSTransition } from 'react-transition-group';

import { GameContext } from '@/GameContext';

import HomeGrid from '@/components/HomeGrid';
import Game from '@/components/Game';

const exo = Exo({ subsets: ['latin', 'latin-ext'] });

export default function Home() {
  const { gameOpts, setGameOpts } = useContext(GameContext);

  const [newGameFormData, setNewGameFormData] = useState(null);
  const [showGame, setShowGame] = useState(false);
  const [showGrid, setShowGrid] = useState(true);

  useEffect(() => {
    console.log(`Home: showGame: ${showGame} showGrid: ${showGrid}`);

    setGameOpts(gameOpts => {{
      return {...gameOpts, setShowGame}
    }})
  }, []);

  useEffect(() => {
    if (showGame && showGrid)
      setShowGrid(false);
    else if (!showGame && !showGrid)
      setShowGrid(true);
  }, [showGame]);

  return (
    <>
      <Game isVisible={showGame} setShowGame={setShowGame} />
      <HomeGrid
        {...{setShowGrid}}
        isVisible={showGrid}
      />
    </>
  );
    {/*<>
      <main className={styles.main}>
        <div className={styles.header}>
          <div className={`${styles.logo} ${styles.unselectable}`}>
            <h1>g</h1>
            <Image
              ref={logoImgRef}
              className={styles.logoImgSpin}
              priority
              src={'/pokerchip3.png'}
              width={75}
              height={75}
              alt='o'
              onClick={toggleSpin}
            />
            <h1>poker</h1>
          </div>
          <p>current games: {'...'}</p>
        </div>

        <div className={styles.center} id='center'>
          {/*<CSSTransition
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
              isVisible={showGame}
              setShowGame={setShowGame}
            />
          </CSSTransition>*}
          <Game isVisible={showGame} setShowGame={setShowGame} />
          <HomeGrid
            {...{setShowGrid}}
            isVisible={showGrid}
          />
          {/*<CSSTransition
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
              {...{setShowGrid}}
              isVisible={showGrid}
            />
          </CSSTransition>*}
        </div>
      </main>
    </>
  )*/}
}
