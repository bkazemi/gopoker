import { Exo } from 'next/font/google';
import Image from 'next/image';
import styles from '@/styles/Home.module.css';

import React, { useCallback, useContext, useEffect, useState, useRef } from 'react';

import { CSSTransition } from 'react-transition-group';

import { Tooltip } from 'react-tooltip';

import { GameContext } from '@/GameContext';

import HomeGrid from '@/components/HomeGrid';
import Game from '@/components/Game';

const exo = Exo({ subsets: ['latin', 'latin-ext'] });

const UnsupportedDeviceToolTip = ({ isUnsupportedDevice, showGame }) => {
  if (!isUnsupportedDevice || showGame)
    return;

  // unfortunately, I cannot get the ref prop to work on <a>
  //
  // there is no way (that I know of) to open tooltip by default while keeping the
  // native behavior of closing on position changes (grid items opening/closing),
  // so we have to manually click the <a> elem
  setTimeout(() => document.querySelector('#unsupportedDeviceToolTipIcon')?.click(), 0);

  return (
    <>
      <a
        id='unsupportedDeviceToolTipIcon'
        style={{
          display: 'flex',
          padding: '5px',
        }}
        data-tooltip-id="unsupportedDeviceToolTip"
      >
        <Image
          src={'/warningIcon.png'}
          width={29}
          height={29}
          alt={'<UnsupportedDevice warning icon>'}
        />
      </a>
      <Tooltip
        id="unsupportedDeviceToolTip"
        style={{ zIndex: 5 }}
        openOnClick={true}
      >
        <p>
          this device's dimensions are not currently supported.
        </p>
      </Tooltip>
    </>
  );
};

export default function Home() {
  const { gameOpts, setGameOpts } = useContext(GameContext);

  //const [newGameFormData, setNewGameFormData] = useState(null);
  const [isUnsupportedDevice, setIsUnsupportedDevice] = useState(false);
  const [showGame, setShowGame] = useState(false);
  const [showGrid, setShowGrid] = useState(true);

  useEffect(() => {
    console.log(`Home: showGame: ${showGame} showGrid: ${showGrid}`);

    setGameOpts(gameOpts => ({
      ...gameOpts,
      setShowGame,
      goHome: gameOpts.goHome ? false : undefined,
    }));

    const screenWidth = typeof window !== 'undefined' ? window.innerWidth : 0;
    setIsUnsupportedDevice(screenWidth < 1080);
  }, []);

  useEffect(() => {
    if (showGame && showGrid)
      setShowGrid(false);
    else if (!showGame && !showGrid)
      setShowGrid(true);
  }, [showGame, showGrid]);

  return (
    <>
      <UnsupportedDeviceToolTip {...{isUnsupportedDevice, showGame }} />
      <Game
        {...{isUnsupportedDevice, setShowGame}}
        isVisible={showGame}
      />
      <HomeGrid
        {...{setShowGrid}}
        isVisible={showGrid}
      />
    </>
  );
}
