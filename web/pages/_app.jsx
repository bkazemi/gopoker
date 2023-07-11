import React, { useState, useEffect, useCallback, useRef } from 'react';

import Image from 'next/image';
import Head from 'next/head';
import dynamic from 'next/dynamic';

import 'react-tooltip/dist/react-tooltip.css';
import cx from 'classnames';

import { GameProvider } from '@/GameContext';

const Header = dynamic(() => import('@/components/Header'), {
  ssr: false,
});

const UnsupportedDevice = dynamic(() => import('@/components/UnsupportedDevice'), {
  ssr: false,
});

import '@/styles/globals.css'

import homeStyles from '@/styles/Home.module.css';

const MainContent = ({ Component, pageProps, router, isCompactRoom }) => {
  // confirm window exit
  useEffect(() => {
    const handleBeforeUnload = (e) => {
      e.preventDefault();
      e.returnValue = '';

      return '';
    };

    window.addEventListener('beforeunload', handleBeforeUnload);

    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload)
    }
  }, []);

  return <>
    <Header />
    <div
      className={cx(
        homeStyles.center,
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

export default function App({ Component, pageProps, router }) {
  const [isUnsupportedDevice, setIsUnsupportedDevice] = useState(undefined);
  const [isCompactRoom, setIsCompactRoom] = useState(false);
  const [isJSEnabled, setIsJSEnabled] = useState(false);

  const [isReadyForRender, setIsReadyForRender] = useState(false);

  if (!process.env.NEXT_PUBLIC_SHOW_LOG) {
    console.debug = console.warn = console.log = () => {}; // keep console.error
  }

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

    if (!isJSEnabled)
      setIsJSEnabled(true);

    if (!isReadyForRender)
      setIsReadyForRender(true);
  }, [router.pathname]);

  if (!isReadyForRender)
    return;

  return (
    <>
      <Head>
        <title>gopoker - shirkadeh.org</title>
        <meta name="header" content="gopoker webclient" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" href="/favicon.ico" />
      </Head>
      <GameProvider>
        <main
          className={cx(
            homeStyles.main,
            isCompactRoom && homeStyles.compactMain
          )}
        >
          {
            !isJSEnabled
              ? <JSDisabled />
              : isUnsupportedDevice
                  ? <UnsupportedDevice isVisible={true} showHomeBtn={false} />
                  : <MainContent {...{Component, pageProps, router, isCompactRoom}} />
          }
        </main>
      </GameProvider>
    </>
  );
}
