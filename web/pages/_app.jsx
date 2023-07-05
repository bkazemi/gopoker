import React, { useState, useEffect, useCallback, useRef } from 'react';

import Image from 'next/image';
import Head from 'next/head';

import 'react-tooltip/dist/react-tooltip.css';

import { GameProvider } from '@/GameContext';
import UnsupportedDevice from '@/components/UnsupportedDevice';
import Header from '@/components/Header';

import '@/styles/globals.css'

import homeStyles from '@/styles/Home.module.css';

const MainContent = ({ Component, pageProps, router }) => {
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
    <div className={homeStyles.center} id='center'>
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

export default function App({ Component, pageProps, router }) {
  const [isUnsupportedDevice, setIsUnsupportedDevice] = useState(false);

  if (!process.env.NEXT_PUBLIC_SHOW_LOG) {
    console.debug = console.warn = console.log = () => {}; // keep console.error
  }

  // check for bare minimum width
  useEffect(() => {
    const screenWidth = typeof window !== 'undefined' ? window.innerWidth : 0;
    setIsUnsupportedDevice(screenWidth < 375);
  }, []);

  return (
    <>
      <Head>
        <title>gopoker - shirkadeh.org</title>
        <meta name="header" content="gopoker webclient" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" href="/favicon.ico" />
      </Head>
      <GameProvider>
        <main className={homeStyles.main}>
          {
            isUnsupportedDevice
              ? <UnsupportedDevice showHomeBtn={false} />
              : <MainContent {...{Component, pageProps, router}} />
          }
        </main>
      </GameProvider>
    </>
  );
}
