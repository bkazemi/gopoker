import React, { useState, useEffect } from 'react';

import Head from 'next/head';
import dynamic from 'next/dynamic';

import 'react-tooltip/dist/react-tooltip.css';
import cx from 'classnames';

import { GameProvider } from '@/GameContext';

import Header from '@/components/Header';
const UnsupportedDevice = dynamic(() => import('@/components/UnsupportedDevice'), {
  ssr: false,
});

import '@/styles/globals.css';
import homeStyles from '@/styles/Home.module.css';

const MainContent = ({ Component, pageProps, isCompactRoom, router }) => {
  const isInGame = router.pathname === '/room/[roomID]';
  const isHomePage = router.pathname === '/';
  const shouldCenterContent = isHomePage || isInGame;

  // confirm window exit only when in a game room
  useEffect(() => {
    if (!isInGame) return;

    const handleBeforeUnload = (e) => {
      e.preventDefault();
      e.returnValue = '';

      return '';
    };

    window.addEventListener('beforeunload', handleBeforeUnload);

    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload)
    }
  }, [isInGame]);

  return <>
    <Header />
    <div
      className={cx(
        homeStyles.center,
        shouldCenterContent && homeStyles.centeredContent,
        isCompactRoom && homeStyles.compactCenter
      )}
      id='center'
    >
      <Component {...pageProps} />
    </div>
    <footer>
      <span style={{ fontStyle: 'italic' }}>
        v0
      </span>
      &nbsp;|&nbsp;
      <a
        href='https://github.com/bkazemi/gopoker'
        target='_blank'
        rel='noopener noreferrer'
      >
        src
      </a>
    </footer>
  </>
};

const JSDisabled = () => (
  <div
    style={{
      display: 'flex',
      placeItems: 'center',
      placeContent: 'center',
      fontSize: '1.5rem',
      width: '100vw',
      height: '100vh',
      fontFamily: 'monospace',
    }}
  >
    <p>Javascript must be enabled to use this site.</p>
  </div>
);

const toggleLogging = (turnOff, dontLog) => {
  if (turnOff || console.debug === window._debug) {
    if (!dontLog) console.log('%clogging turned off', 'font-style: italic');
    console.debug = console.warn = console.log = () => {}; // keep console.error
  } else {
    console.debug = window._debug;
    console.warn  = window._warn;
    console.log   = window._log;
    if (!dontLog) console.log('%clogging turned on', 'font-style: italic');
  }
};

export default function App({ Component, pageProps, router }) {
  const [isUnsupportedDevice, setIsUnsupportedDevice] = useState(undefined);
  const [isCompactRoom, setIsCompactRoom] = useState(false);

  // toggle logging for debugging
  useEffect(() => {
    const handleKeyDown = (event) => {
      if (window._debug === undefined)
        return;

      if (event.ctrlKey && event.key === 'F7') {
        toggleLogging();
      }
    };

    if (window._debug === undefined) {
      window._debug = console.debug;
      window._warn  = console.warn;
      window._log   = console.log;
    }

    if (!process.env.NEXT_PUBLIC_SHOW_LOG)
      toggleLogging(true, true);

    window.addEventListener('keydown', handleKeyDown);

    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, []);

  // check for bare minimum screen width and a small screen size
  // (which if true will render a smaller game room stylesheet)
  //
  // I also treat this like a useEffect with empty deps because that's basically what it is;
  // then I can set the variables that depend on window before the first render.
  useEffect(() => {
    setIsUnsupportedDevice(window?.screen?.width < 375);
    // hackish, but to avoid using GameContext inside App I made roomURL a
    // global variable
    setIsCompactRoom(router.pathname === '/room/[roomID]'
      && window.roomURL && window?.innerWidth <= 1920);
  }, [router.pathname]);

  return (
    <>
      <Head>
        <title>gopoker - shirkadeh.org</title>
        <meta name="header" content="gopoker webclient" />
        <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover" />
        <link rel="icon" href="/favicon.ico" />
      </Head>
      <GameProvider>
        <main
          className={cx(
            homeStyles.main,
            isCompactRoom && homeStyles.compactMain
          )}
        >
          <div className="appShell">
            {
              isUnsupportedDevice
                ? <UnsupportedDevice isVisible={true} showHomeBtn={false} />
                : <MainContent {...{Component, pageProps, router, isCompactRoom}} />
            }
          </div>
          <noscript><JSDisabled /></noscript>
        </main>
      </GameProvider>
    </>
  );
}
