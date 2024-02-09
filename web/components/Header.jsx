import React, { useState, useCallback, useRef, useContext, useEffect } from 'react';

import { M_PLUS_Code_Latin } from 'next/font/google';
import Image from 'next/image';
import { useRouter } from 'next/router';

import cx from 'classnames';

import { GameContext } from '@/GameContext';

import homeStyles from '@/styles/Home.module.css';

const mPlus = M_PLUS_Code_Latin({ subsets: ['latin'], weight: '700' });

export default function Header({ isTableHeader }) {
  const {gameOpts, setGameOpts} = useContext(GameContext);

  const logoImgRef = useRef(null);

  const router = useRouter();

  const [headerInfo, setHeaderInfo] = useState('fetching...');
  const [headerError, setHeaderError] = useState(false);
  const [headerChipLoaded, setHeaderChipLoaded] = useState(false);

  const [isHomePage, setIsHomePage] = useState(router.pathname === '/');

  const [windowWidth, setWindowWidth] = useState(window?.innerWidth);
  const [isCompactRoom, setIsCompactRoom] =
    useState(router.pathname === '/room/[roomID]' && gameOpts.roomURL
      && window?.innerWidth <= 1920);

  useEffect(() => {
    const handleResize = () => setWindowWidth(window.innerWidth);

    window.addEventListener('resize', handleResize)

    return () => window.removeEventListener('resize', handleResize);
  }, []);

  const fetchHeaderInfo = useCallback(async (pathname) => {
    const isHomePage = pathname === '/';
    const URL = `/api/${isHomePage ? 'status' : 'roomCount'}`;

    setIsHomePage(isHomePage);

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
      setHeaderError(true);
    }
  }, []);

  const toggleSpin = useCallback(() => {
    if (logoImgRef.current?.classList.contains(homeStyles.pauseAnimation))
      logoImgRef.current?.classList.remove(homeStyles.pauseAnimation);
    else
      logoImgRef.current?.classList.add(homeStyles.pauseAnimation);
  }, [logoImgRef]);

  useEffect(() => {
    fetchHeaderInfo(router.pathname);
  }, [router.pathname, fetchHeaderInfo]);

  useEffect(() => {
    const isConnectedRoom = router.pathname === '/room/[roomID]' && gameOpts.roomURL;
    const winWidth = windowWidth || window.innerWidth;

    setIsCompactRoom(isConnectedRoom && winWidth <= 1920);
  }, [router.pathname, gameOpts.roomURL, windowWidth]);

  if ((isCompactRoom && !isTableHeader) || (!isCompactRoom && isTableHeader))
    return;

  return (
    <div className={cx(
        homeStyles.header,
        isCompactRoom && homeStyles.compactHeader
      )}
    >
      <div
        className={cx(homeStyles.logo, mPlus.className, homeStyles.unselectable)}
        style={{
          opacity: headerChipLoaded ? 1 : 0
        }}
      >
        <h1>g</h1>
        <Image
          ref={logoImgRef}
          priority
          src={'/pokerchip3.png'}
          width={75}
          height={75}
          alt='o'
          onClick={toggleSpin}
          onLoad={() => setHeaderChipLoaded(true)}
        />
        <h1>poker</h1>
      </div>
      <p>
        { isHomePage ? 'server status:' : 'current games:' }
        &nbsp;
        <span
          style={{
            color: headerError ? 'red' : isHomePage ? 'green' : 'inherit'
          }}
        >
          { headerInfo }
        </span>
      </p>
    </div>
  );
}
