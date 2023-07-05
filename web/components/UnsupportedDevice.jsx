import React, { useCallback, useContext } from 'react';

import { useRouter } from 'next/router';

import { Literata } from 'next/font/google';

import cx from 'classnames';

const literata = Literata({ subsets: ['latin'], weight: '500' })

import { GameContext } from '@/GameContext';

import styles from '@/styles/UnsupportedDevice.module.css';

function UnsupportedDevice({ isVisible, showHomeBtn }) {
  const router = useRouter();

  const {gameOpts, setGameOpts} = useContext(GameContext);

  const { setShowGame } = gameOpts;

  const goHome = useCallback(() => {
    console.log('goHome()');
    setGameOpts(opts => ({
      ...opts,
      goHome: true,
    }));

    if (router.pathname === '/') // came from NewGameForm
      setShowGame(false);
    else // came from a /room route
      router.push('/');
  }, []);

  if (!isVisible)
    return;

  return (
    <div className={cx(styles.container, literata.className)}>
      <h1>Your device's dimensions are not currently supported</h1>
      { showHomeBtn && <button onClick={goHome}>go home</button> }
    </div>
  );
}

export default React.memo(UnsupportedDevice);
