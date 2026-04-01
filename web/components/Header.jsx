import React, { useState, useCallback, useRef, useContext, useEffect } from 'react';

import { M_PLUS_Code_Latin } from 'next/font/google';
import Image from 'next/image';
import { useRouter } from 'next/router';

import cx from 'classnames';

import { GameContext } from '@/GameContext';

import useDeferredLoading from '@/lib/useDeferredLoading';

import homeStyles from '@/styles/Home.module.css';

const mPlus = M_PLUS_Code_Latin({ subsets: ['latin'], weight: '700' });

export default function Header({ isTableHeader }) {
  const { gameOpts } = useContext(GameContext);

  const router = useRouter();
  const isHomePage = router.pathname === '/';

  const [headerInfo, setHeaderInfo] = useState(null);
  const [headerError, setHeaderError] = useState(false);
  const showHeaderLoading = useDeferredLoading(headerInfo === null);
  const [headerChipLoaded, setHeaderChipLoaded] = useState(false);

  const [windowWidth, setWindowWidth] = useState(
    typeof window !== 'undefined' ? window.innerWidth : 0
  );
  const isCompactRoom = router.pathname === '/room/[roomID]'
    && !!gameOpts.roomURL
    && windowWidth <= 1920;

  useEffect(() => {
    const handleResize = () => setWindowWidth(window.innerWidth);

    window.addEventListener('resize', handleResize);

    return () => window.removeEventListener('resize', handleResize);
  }, []);

  const fetchHeaderInfo = useCallback(async (pathname) => {
    const isHomePage = pathname === '/';
    const URL = `/api/${isHomePage ? 'status' : 'roomCount'}`;

    setHeaderInfo(null);
    setHeaderError(false);

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

  const logoImgRef = useRef(null);
  const shouldRenderHeader = !!isCompactRoom === !!isTableHeader; // both true or both false

  const toggleSpin = useCallback(() => {
    logoImgRef.current?.classList.toggle(homeStyles.pauseAnimation);
  }, []);

  useEffect(() => {
    fetchHeaderInfo(router.pathname);
  }, [router.pathname, fetchHeaderInfo]);

  if (!shouldRenderHeader)
    return null;

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
        <div className={homeStyles.logoChip}>
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
        </div>
        <h1>poker</h1>
      </div>
      <p style={{
        visibility: headerInfo !== null || showHeaderLoading ? 'visible' : 'hidden'
      }}>
        { isHomePage ? 'server status:' : 'current games:' }
        &nbsp;
        <span
          style={{
            color: headerError ? 'red' : isHomePage ? 'green' : 'inherit'
          }}
        >
          { headerInfo !== null ? headerInfo : 'fetching...' }
        </span>
      </p>
    </div>
  );
}
