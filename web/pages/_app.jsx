import React, { useState, useEffect, useCallback, useRef } from 'react';

import Image from 'next/image';
import Head from 'next/head';

import { GameProvider } from '@/GameContext';

import '@/styles/globals.css'

import homeStyles from '@/styles/Home.module.css';

export default function App({ Component, pageProps, router }) {
  const logoImgRef = useRef(null);

  const [headerInfo, setHeaderInfo] = useState('fetching...');

  const isHomePage = router.pathname === '/';

  const fetchHeaderInfo = useCallback(async () => {
    const URL = `/api/${isHomePage ? 'status' : 'roomCount'}`;

    try {
      const res = await fetch(URL);
      if (res.ok) {
        const info = await res.json();
        setHeaderInfo(isHomePage ? info.status : info.roomCount);
      } else {
        throw new Error();
      }
    } catch (e) {
      setHeaderInfo(isHomePage ? 'down' : 'error');
    }
  }, [router.pathname]);

  const toggleSpin = useCallback(() => {
    if (logoImgRef.current.classList.contains(homeStyles.pauseAnimation))
      logoImgRef.current.classList.remove(homeStyles.pauseAnimation);
    else
      logoImgRef.current.classList.add(homeStyles.pauseAnimation);
  }, [logoImgRef]);

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

  fetchHeaderInfo();

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
          <div className={homeStyles.header}>
            <div className={`${homeStyles.logo} ${homeStyles.unselectable}`}>
              <h1>g</h1>
              <Image
                ref={logoImgRef}
                className={homeStyles.logoImgSpin}
                priority
                src={'/pokerchip3.png'}
                width={75}
                height={75}
                alt='o'
                onClick={toggleSpin}
              />
              <h1>poker</h1>
            </div>
            {
              <p>
                { isHomePage ?
                  'server status: ' + headerInfo
                  :
                  'current games: ' + headerInfo
                }
              </p>
            }
          </div>
          <div className={homeStyles.center} id='center'>
            <Component {...pageProps} />
          </div>
        </main>
      </GameProvider>
    </>
  );
}
