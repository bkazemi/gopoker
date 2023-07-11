import React, { useState, useCallback, useRef, useContext, useEffect } from 'react';

import Image from 'next/image';
import { useRouter } from 'next/router';

import cx from 'classnames';

import { GameContext } from '@/GameContext';

import homeStyles from '@/styles/Home.module.css';

export default function Header({ isTableHeader }) {
  const {gameOpts, setGameOpts} = useContext(GameContext);

  const logoImgRef = useRef(null);

  const router = useRouter();

  const [headerInfo, setHeaderInfo] = useState('fetching...');
  const [headerError, setHeaderError] = useState(false);

  const [isHomePage, setIsHomePage] = useState(router.pathname === '/');

  const [isCompactRoom, setIsCompactRoom] =
    useState(router.pathname === '/room/[roomID]' && gameOpts.roomURL
      && window?.innerWidth <= 1920);

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
      setHeaderError(true);
    }
  }, [isHomePage]);

  const toggleSpin = useCallback(() => {
    if (logoImgRef.current?.classList.contains(homeStyles.pauseAnimation))
      logoImgRef.current?.classList.remove(homeStyles.pauseAnimation);
    else
      logoImgRef.current?.classList.add(homeStyles.pauseAnimation);
  }, [logoImgRef]);

  useEffect(() => {
    setIsHomePage(router.pathname === '/');
    fetchHeaderInfo();
  }, [router.pathname, fetchHeaderInfo]);

  useEffect(() => {
    const isConnectedRoom = router.pathname === '/room/[roomID]' && gameOpts.roomURL;

    setIsCompactRoom(isConnectedRoom && window.innerWidth <= 1920);
  }, [router.pathname, gameOpts.roomURL]);

  if ((isCompactRoom && !isTableHeader) || (!isCompactRoom && isTableHeader))
    return;

  return (
    <div className={cx(
        homeStyles.header,
        isCompactRoom && homeStyles.compactHeader
      )}
    >
      <div className={cx(homeStyles.logo, homeStyles.unselectable)}>
        <h1>g</h1>
        <Image
          ref={logoImgRef}
          priority
          src={'/pokerchip3.png'}
          width={75}
          height={75}
          alt='o'
          onClick={toggleSpin}
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
