import React, { useEffect, useState } from 'react';

import { useRouter } from 'next/router';
import { Exo } from 'next/font/google';
import { Literata } from 'next/font/google';

const exo = Exo({ subsets: ['latin'] });
const literata = Literata({ subsets: ['latin'], weight: '500' });

import styles from '@/styles/RoomList.module.css';

const RoomInfo = ({ isVisible, room }) => {
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
};

const RoomListItem = ({ room }) => {
  const [clicked, setClicked] = useState(false);

  const router = useRouter();

  const roomLink = `/room/${room.roomName}`;

  return (
  <div
    className={styles.roomListItem}
    onClick={() => setClicked(!clicked)}
  >
    <div style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between'
    }}>
      <p
        style={{ opacity: '0.9' }}
        className={literata.className}
      >
        { room.roomName }
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
};

export default function RoomList({ isVisible }) {
  const [curRoomCnt, setCurRoomCnt] = useState('fetching room count...');

  const [roomList, setRoomList] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    const fetchRoomList = async () => {
      try {
        const listRes = await fetch('/api/roomList');
        if (listRes.ok) {
          setRoomList(await listRes.json());
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
        setCurRoomCnt('unknown');
      }
    };

    if (isVisible)
      fetchRoomList();
    else
      fetchCurRoomCnt();
  }, [isVisible]);

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
    <div
      className={isVisible ? styles.roomList : 'hidden'}
      onClick={(e) => { e.stopPropagation() }}
    >
    {
      roomList.length &&
      roomList
        .map((room, idx) => {
          return <RoomListItem key={idx} room={room} />
        }) ||
      <p className={exo.className}>there are no rooms presently</p>
    }
    </div>
  );
}
