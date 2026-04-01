import React from 'react';

import { Exo } from 'next/font/google';

import RoomList from '@/components/RoomList';

import styles from '@/styles/RoomList.module.css';

const exo = Exo({ subsets: ['latin', 'latin-ext'] });

export default function Rooms() {
  return (
    <div className={styles.roomsPage}>
      <h2 className={exo.className} style={{ margin: 0 }}>
        public rooms
      </h2>
      <RoomList isVisible={true} mode='page' />
    </div>
  );
}
