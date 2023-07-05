import React, { useState, useCallback, useRef } from 'react';

import Image from 'next/image';
import { useRouter } from 'next/router';

import homeStyles from '@/styles/Home.module.css';

export default function Header() {
  const logoImgRef = useRef(null);

  const [headerInfo, setHeaderInfo] = useState('fetching...');
  const [headerError, setHeaderError] = useState(false);

  const router = useRouter();

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
      setHeaderError(true);
    }
  }, [router.pathname]);

  const toggleSpin = useCallback(() => {
    if (logoImgRef.current.classList.contains(homeStyles.pauseAnimation))
      logoImgRef.current.classList.remove(homeStyles.pauseAnimation);
    else
      logoImgRef.current.classList.add(homeStyles.pauseAnimation);
  }, [logoImgRef]);

  fetchHeaderInfo();

  return (
    <div className={homeStyles.header}>
      <div className={`${homeStyles.logo} ${homeStyles.unselectable}`}>
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
      {
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
      }
    </div>
  );
}
