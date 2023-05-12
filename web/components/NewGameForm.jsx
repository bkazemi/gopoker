import React, { useEffect, useState } from 'react';
import Select from 'react-select';

import { Literata } from 'next/font/google';

import styles from '@/styles/NewGameForm.module.css';
import { NETDATA, TABLE_LOCK, NetData, NewClient } from '@/lib/libgopoker';


const literata = Literata({
  subsets: ['latin'],
  weight: '500',
});

const lockOpts = [
  { value: TABLE_LOCK.NONE,       label: 'none'                    },
  { value: TABLE_LOCK.PLAYERS,    label: 'player lock'             },
  { value: TABLE_LOCK.SPECTATORS, label: 'spectator lock'          },
  { value: TABLE_LOCK.ALL,        label: 'player & spectator lock' },
];

const maxPlayerOpts = [
  { value: 2, label: '2'},
  { value: 3, label: '3'},
  { value: 4, label: '4'},
  { value: 5, label: '5'},
  { value: 6, label: '6'},
  { value: 7, label: '7'},
];

export default function NewGameForm({ isVisible, isSettings, setFormData}) {
  const [tableLock, setTableLock] = useState(TABLE_LOCK.NONE);
  const [tablePwd , setTablePwd] = useState('');
  const [maxPlayers, setMaxPlayers] = useState(7);

  const handleSubmit = async (event) => {
    event.preventDefault();

    //const Name = event.target.tableName.value,
      const Name = event.target.playerName.value;
      const Password = tablePwd;
      const TableLock = tableLock;

    const data = new NetData(
      NewClient({
        Name,
        Password,
        TableLock,
        TablePass: Password
      }),
      isSettings ? NETDATA.CLIENT_SETTINGS : NETDATA.NEWCONN,
    );

    setFormData(data);
  };

  return (
    <div
      className={isVisible ? styles.newGameForm : 'hidden'}
      onClick={(e) => { e.stopPropagation() }}
    >
      {/*<form action="/new" method="post" className={literata.className}>*/}
      <form className={literata.className} onSubmit={handleSubmit}>
        <label htmlFor="tableName">table name</label>
        <input
          type="text"
          id="tableName"
          name="tableName"
          required
          minLength="1"
          maxLength="50"
        />
        <label htmlFor='playerName' onSubmit={handleSubmit}>player name</label>
        <input
          type='text'
          id='playerName'
          name='playerName'
        />
        <label htmlFor='tablePwd'>password</label>
        <input
          type='password'
          id='tablePwd'
          name='tablePwd'
          onClick={(e) => {
            e.target.type = e.target.type === 'password' ? 'text' : 'password';
          }}
        />
        <label>table lock</label>
        <Select
          options={lockOpts}
          inputId='tableLock'
          onChange={(sel) => setTableLock(sel.value)}
        />
        <label>maximum seats</label>
        <Select
          options={maxPlayerOpts}
          inputId='maxPlayers'
          onChange={sel => setMaxPlayers(sel.value)}
        />
        <button type="submit">submit</button>
      </form>
    </div>
  );
}
