import React, { useContext, useEffect, useState, useRef } from 'react';

import Select from 'react-select';

import Image from 'next/image';
import { useRouter } from 'next/router';
import { Literata } from 'next/font/google';

import { GameContext } from '@/GameContext';
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

const RequiredFields = ({
  router,
  isSettings, isDirectLink,
  roomName, name, tablePwd, tableLock, maxPlayers, tablePwdRef,
  handleSubmit, setModalOpen, setRoomName, setName, setTablePwd, setTableLock, setMaxPlayers
}) => (
  <>
    {
      !isDirectLink &&
      <>
        <label htmlFor="roomName">room name</label>
        <input
          type="text"
          id="roomName"
          name="roomName"
          required
          minLength="1"
          maxLength="50"
          value={roomName}
          onChange={(e) => setRoomName(e.target.value)}
        />
      </>
    }

    <label htmlFor='playerName' onSubmit={handleSubmit}>player name</label>
    <input
      type='text'
      id='playerName'
      name='playerName'
      value={name}
      onChange={(e) => setName(e.target.value)}
    />
    <label htmlFor='tablePwd'>password</label>
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'auto min-content',
        gap: '5px',
        alignItems: 'center',
      }}
    >
      <input
        ref={tablePwdRef}
        type='password'
        id='tablePwd'
        name='tablePwd'
        value={tablePwd}
        onChange={(e) => setTablePwd(e.target.value)}
      />
      <Image
        style={{
          cursor: 'pointer'
        }}
        priority
        src={'/eye2.png'}
        width={33}
        height={33}
        alt='[show password]'
        onClick={() => {
          if (tablePwdRef.current)
            tablePwdRef.current.type = tablePwdRef.current.type === 'password' ? 'text' : 'password';
        }}
      />
    </div>

    {
      !isDirectLink &&
      <>
        <label>table lock</label>
        <Select
          options={lockOpts}
          inputId='tableLock'
          value={tableLock}
          onChange={sel => setTableLock(sel)}
        />
        <label>maximum seats</label>
        <Select
          options={maxPlayerOpts}
          inputId='maxPlayers'
          value={maxPlayers}
          onChange={sel => setMaxPlayers(sel)}
        />
      </>
    }

    <div className={styles.formBtns}>
      <button type="submit">{ isDirectLink ? 'connect' : 'submit' }</button>
      <button
        onClick={() => {
          if (isSettings) setModalOpen(false)
          else router.push('/')
        }}
      >
        { isSettings ? 'cancel' : 'go home' }
      </button>
    </div>
  </>
);

export default function NewGameForm({ isVisible, isSettings, isDirectLink, setModalOpen }) {
  const {gameOpts, setGameOpts} = useContext(GameContext);

  const { Name, Password } = gameOpts.websocketOpts?.Client?.Settings || {Name: '', Password: ''};
  const { RoomName, Lock } = gameOpts.websocketOpts?.Client?.Settings?.Admin || {RoomName: '', Lock: null};

  const router = useRouter();

  const tablePwdRef = useRef(null);

  const [roomName, setRoomName] = useState(RoomName);
  const [name, setName] = useState(Name);
  const [tablePwd , setTablePwd] = useState(Password);
  const [tableLock, setTableLock] = useState(lockOpts.find(opt => opt.value === Lock) || lockOpts[0]);
  const [maxPlayers, setMaxPlayers] = useState(maxPlayerOpts.find(opt => opt.value === Lock)); // TODO: implement

  const handleSubmit = async (event) => {
    event.preventDefault();

    const RoomName = roomName;
    const Name = name;
    const Password = tablePwd;
    const TableLock = tableLock.value;

    const data = new NetData(
      NewClient({
        Name,
        Password,
        RoomName,
        TableLock,
        TablePass: Password
      }),
      isSettings ? NETDATA.CLIENT_SETTINGS : NETDATA.NEWCONN,
    );

    setGameOpts(opts => {
      const newOpts = {...opts, websocketOpts: data};

      return isSettings ? {...newOpts, settingsChange: true} : newOpts;
    });

    setModalOpen && setModalOpen(false);

    if (!isSettings && !isDirectLink) {
      gameOpts.setShowGame(true);
    }
  };

  return (
    <div
      className={isVisible ? styles.newGameForm : 'hidden'}
      onClick={(e) => { e.stopPropagation() }}
    >
      {/*<form action="/new" method="post" className={literata.className}>*/}
      <form
        className={literata.className}
        onSubmit={handleSubmit}
        style={isDirectLink && { minWidth: 0 }}
      >
        <RequiredFields
          {...{
            router,
            isSettings, isDirectLink, handleSubmit,
            roomName, name, tablePwd, tableLock, maxPlayers, tablePwdRef,
            setModalOpen, setRoomName, setName, setTablePwd, setTableLock, setMaxPlayers
          }}
        />
      </form>
    </div>
  );
}
