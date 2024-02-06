import React, { useEffect, useState, useRef } from 'react';

import { useRouter } from 'next/router';
import { Exo } from 'next/font/google';
import { Literata } from 'next/font/google';

const exo = Exo({ subsets: ['latin', 'latin-ext'], });
const literata = Literata({ subsets: ['latin', 'latin-ext'], weight: '500' });

import { TABLE_LOCK } from '@/lib/libgopoker';

import styles from '@/styles/RoomList.module.css';

const RoomInfo = React.memo(({ isVisible, room }) => {
  if (!isVisible)
    return;

  return (
    <div className={styles.roomListInfo}>
      <p># connected: {room.numConnected}</p>
      <p># seats: {room.numSeats}</p>
      <p># current players: {room.numPlayers}</p>
      <p># open seats: {room.numOpenSeats}</p>
      <p>table lock: {room.tableLock}</p>
      <p>password protected: {room.needPassword ? 'yes' : 'no'}</p>
    </div>
  );
});

RoomInfo.displayName = 'RoomInfo';

const RoomListItem = React.memo(({ room, searchRegex, roomListRef }) => {
  const roomListItemRef = useRef(null);

  const [clicked, setClicked] = useState(false);
  const prevScrollPos = useRef(0);

  const router = useRouter();

  const roomLink = `/room/${room.roomName}`;

  useEffect(() => {
    console.log('roomListItem clicked');
    if (clicked && roomListRef.current && roomListItemRef.current) {
      prevScrollPos.current = roomListRef.current.scrollTop;
      console.log('scrolling to roomListItem')
      roomListItemRef.current.scrollIntoView({
        behavior: 'smooth',
        block: 'nearest',
      })
    } else {
      console.log(`scrolling to pos: ${prevScrollPos.current}`);
      roomListRef.current.scrollTo({
        top: prevScrollPos.current,
        behavior: 'smooth',
      })
    }
  }, [clicked, roomListRef]);

  return (
  <div
    ref={roomListItemRef}
    className={styles.roomListItem}
    onClick={() => setClicked(!clicked)}
  >
    <div
      className={literata.className}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between'
      }}
    >
      <p style={{ opacity: '0.9' }}>
      {
        searchRegex &&
        room.roomName
          .split(searchRegex)
          .reduce((acc, char) => {
            if (acc.length === 0 || acc[acc.length - 1] !== char)
              acc.push(char);
            else
              acc[acc.length - 1] += char;

            return acc;
          }, [])
          .map((part, idx) => {
            return (
              searchRegex.test(part) ?
              <span key={idx} className={styles.searchHighlight}>
                { part }
              </span>
              : part
            );
          })
        ||
        room.roomName
      }
      </p>
      <button
        style={{
          padding: '5px',
        }}
        onClick={(e) => {
          e.stopPropagation();
          router.push(roomLink);
        }}
      >
        join
      </button>
    </div>
    <RoomInfo
      isVisible={clicked}
      room={room}
    />
  </div>
  );
});

RoomListItem.displayName = 'RoomListItem';

function RoomList({ isVisible }) {
  const [curRoomCnt, setCurRoomCnt] = useState('fetching room count...');

  const [roomList, setRoomList] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);
  const [searchValue, setSearchValue] = useState('');
  const [searchRegex, setSearchRegex] = useState(null);

  const roomListRef = useRef(null);

  useEffect(() => {
    const fetchRoomList = async () => {
      try {
        const listRes = await fetch('/api/roomList');
        if (listRes.ok) {
          const roomList = (await listRes.json())
            .sort((a, b) => a.roomName.localeCompare(b.roomName))
            .map(room => {
              return {
                ...room,
                tableLock: TABLE_LOCK.toString(room.tableLock),
              }
          });
          setRoomList(roomList);
        } else {
          throw new Error();
        }
      } catch (e) {
        setError(true);
        setIsLoading(false);
      }
    };
    const fetchCurRoomCnt = async () => {
      try {
        const roomCntRes = await fetch('/api/roomCount');

        if (roomCntRes.ok) {
          const roomCnt = await roomCntRes.json();
          setCurRoomCnt(roomCnt.roomCount);
          setIsLoading(false);
        } else {
          throw new Error();
        }
      } catch (e) {
        setCurRoomCnt('N/A');
      }
    };

    if (isVisible)
      fetchRoomList();
    else
      fetchCurRoomCnt();
  }, [isVisible]);

  useEffect(() => {
    setSearchRegex(searchValue ? new RegExp(`(${searchValue})`, 'gi') : null);
  }, [searchValue]);

  if (!isVisible) {
    return (
      <p className={exo.className}>
        current games: { curRoomCnt }
      </p>
    );
  }

  if (isLoading) {
    return (
      <p className={exo.className}>fetching room list...</p>
    );
  }

  if (error) {
    return (
      <p className={exo.className}>error fetching room list</p>
    );
  }

  return (
    <>
      <div
        className={styles.search}
        onClick={e => e.stopPropagation()}
      >
        <label
          className={exo.className}
        >
          search
        </label>
        <input
          onChange={(e) => setSearchValue(e.target.value)}
        />
      </div>
      <div
        ref={roomListRef}
        className={isVisible ? styles.roomList : 'hidden'}
        onClick={(e) => e.stopPropagation()}
      >
      {
        roomList.length &&
        roomList
          .filter(room => !searchRegex || searchRegex.test(room.roomName))
          .map((room, idx) => {
            return <RoomListItem
                     key={idx}
                     {...{room, searchRegex, roomListRef}}
                   />
          }) ||
        <p className={exo.className}>there are currently no rooms</p>
      }
      </div>
    </>
  );
}

RoomList.displayName = 'RoomList';

export default React.memo(RoomList);
