import React, { useContext, useEffect, useState, useRef, useCallback } from 'react';

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

const RequiredFields = React.memo(({
  goHome,
  isSettings, isDirectLink, isAdmin, isSpectatorChecked,
  roomName, name, tablePwd, tableLock, maxPlayers, tablePwdRef,
  handleSubmit, setModalOpen, setRoomName, setName, setTablePwd, setTableLock,
  setMaxPlayers, setIsSpectatorChecked
}) => (
  <>
    {
      !isDirectLink && (!isSettings || (isSettings && isAdmin)) &&
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
            tablePwdRef.current.type = tablePwdRef.current.type === 'password'
              ? 'text' : 'password';
        }}
      />
    </div>

    {
      !isDirectLink && (!isSettings || (isSettings && isAdmin)) &&
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

    {
      !isSettings &&
      <label>
        <input
          type='checkbox'
          checked={isSpectatorChecked}
          onChange={e => setIsSpectatorChecked(e.target.checked)}
          style={{
            marginRight: '5px',
            scale: '1.1',
          }}
        />
        join as spectator
      </label>
    }

    <div className={styles.formBtns}>
      <button type="submit">
        { isDirectLink ? 'connect' : 'submit' }
      </button>
      <button
        onClick={() => {
          if (isSettings) setModalOpen(false)
          else goHome();
        }}
      >
        { isSettings ? 'cancel' : 'go home' }
      </button>
    </div>
  </>
));

RequiredFields.displayName = 'RequiredFields';

function NewGameForm({ isVisible, isSettings, isDirectLink, setModalOpen }) {
  const {gameOpts, setGameOpts} = useContext(GameContext);

  const { Name, Password } = gameOpts.websocketOpts?.Client?.Settings || {Name: '', Password: ''};
  const { RoomName, Lock, NumSeats } = gameOpts.websocketOpts?.Client?.Settings?.Admin || {RoomName: '', Lock: null, NumSeats: 7};

  const router = useRouter();

  const newGameFormRef = useRef(null);
  const tablePwdRef = useRef(null);

  const [roomName, setRoomName] = useState(RoomName);
  const [name, setName] = useState(Name);
  const [tablePwd , setTablePwd] = useState(Password);
  const [tableLock, setTableLock] = useState(lockOpts.find(opt => opt.value === Lock) || lockOpts[0]);
  const [maxPlayers, setMaxPlayers] = useState(maxPlayerOpts.find(opt => opt.value === NumSeats) || maxPlayerOpts[0]);
  const [isSpectatorChecked, setIsSpectatorChecked] = useState(false);

  const isAdmin = !!gameOpts.isAdmin;

  const goHome = useCallback(() => {
    console.log('goHome()');
    setGameOpts(opts => ({
      ...opts,
      goHome: true,
    }));

    router.push('/');
  }, [setGameOpts, router]);

  const handleSubmit = async (event) => {
    event.preventDefault();

    const IsSpectator = isSpectatorChecked;
    const RoomName = roomName;
    const Name = name;
    const Password = tablePwd;
    const TableLock = tableLock.value;
    const TableNumSeats = maxPlayers.value;

    const data = new NetData(
      NewClient({
        IsSpectator,
        Name,
        Password,
        RoomName,
        TableLock,
        TableNumSeats,
        TablePass: Password,
      }),
      isSettings ? NETDATA.CLIENT_SETTINGS : NETDATA.NEWCONN,
    );

    setGameOpts(opts => {
      const newOpts = {...opts, websocketOpts: data, reset: false};

      return isSettings ? {...newOpts, settingsChange: true} : newOpts;
    });

    setModalOpen && setModalOpen(false);

    if (!isSettings && !isDirectLink) {
      gameOpts.setShowGame(true);
    }
  };

  useEffect(() => {
    if (isVisible && newGameFormRef.current) {
      console.log('scrolling into newGameForm')
      newGameFormRef.current.parentNode.scrollIntoView({
          behavior: 'smooth',
          block: 'nearest',
      });
    }
  }, [isVisible, newGameFormRef]);

  useEffect(() => {
    if (gameOpts.reset) {
      setRoomName('');
      setName('');
      setTablePwd('');
      setTableLock(lockOpts[0]);
      setMaxPlayers(7);
      setIsSpectatorChecked(false);
    }
  }, [gameOpts.reset]);

  return (
    <div
      ref={newGameFormRef}
      className={isVisible ? styles.newGameForm : 'hidden'}
      onClick={(e) => e.stopPropagation()}
    >
      {/*<form action="/new" method="post" className={literata.className}>*/}
      <form
        className={literata.className}
        onSubmit={handleSubmit}
        style={isDirectLink && { minWidth: 0 }}
      >
        <RequiredFields
          {...{
            goHome,
            isSettings, isDirectLink, isAdmin, isSpectatorChecked, handleSubmit,
            roomName, name, tablePwd, tableLock, maxPlayers, tablePwdRef,
            setModalOpen, setRoomName, setName, setTablePwd, setTableLock,
            setMaxPlayers, setIsSpectatorChecked
          }}
        />
      </form>
    </div>
  );
}

NewGameForm.displayName = 'NewGameForm';

export default React.memo(NewGameForm);
