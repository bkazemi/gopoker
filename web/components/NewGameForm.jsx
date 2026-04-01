import React, { useContext, useEffect, useState, useRef, useCallback } from 'react';

import Image from 'next/image';
import { useRouter } from 'next/router';
import { Literata } from 'next/font/google';

import Select from 'react-select';

import { GameContext } from '@/GameContext';
import { NETDATA, TABLE_LOCK, NetData, NewClient, NewRoomSettings } from '@/lib/libgopoker';

import styles from '@/styles/NewGameForm.module.css';

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
  roomName, name, passwordLabel, tablePwd, tableLock, maxPlayers, tablePwdRef,
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
    <label htmlFor='tablePwd'>{ passwordLabel }</label>
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

  const clientSettings = gameOpts.websocketOpts?.Client?.Settings || {Name: '', Password: ''};
  const roomSettings = gameOpts.roomSettings || {RoomName: '', Lock: null, NumSeats: 7, Password: ''};
  const { Name } = clientSettings;
  const { RoomName, Lock, NumSeats } = roomSettings;

  const router = useRouter();

  const newGameFormRef = useRef(null);
  const tablePwdRef = useRef(null);

  const [roomName, setRoomName] = useState(RoomName);
  const [name, setName] = useState(Name);
  const [tablePwd , setTablePwd] = useState(
    isDirectLink || (isSettings && !gameOpts.isAdmin)
      ? (clientSettings.Password || '')
      : (roomSettings.Password || '')
  );
  const [tableLock, setTableLock] = useState(lockOpts.find(opt => opt.value === Lock) || lockOpts[0]);
  const [maxPlayers, setMaxPlayers] = useState(maxPlayerOpts.find(opt => opt.value === NumSeats) || maxPlayerOpts[0]);
  const [isSpectatorChecked, setIsSpectatorChecked] = useState(false);

  const isAdmin = !!gameOpts.isAdmin;
  const passwordLabel = (!isDirectLink && !isSettings) || (isSettings && isAdmin)
    ? 'table password'
    : 'password';

  const goHome = useCallback(() => {
    console.log('goHome()');
    setGameOpts(opts => ({
      ...opts,
      creatorToken: undefined,
      creatorTokenRoomID: undefined,
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
    const nextRoomSettings = NewRoomSettings({
      RoomName,
      Lock: TableLock,
      NumSeats: TableNumSeats,
      Password: isDirectLink || (isSettings && !isAdmin) ? roomSettings.Password : Password,
    });

    let clientPassword = '';
    if (isDirectLink || (isSettings && !isAdmin))
      clientPassword = Password;
    else if (isSettings && isAdmin)
      clientPassword = clientSettings.Password || '';

    const client = NewClient({
      IsSpectator,
      Name,
      Password: clientPassword,
    });
    const data = new NetData(
      client,
      isSettings
        ? (isAdmin ? NETDATA.ADMIN_SETTINGS : NETDATA.CLIENT_SETTINGS)
        : NETDATA.NEWCONN,
      "",
      null,
      isSettings && isAdmin ? nextRoomSettings : null,
    );

    setGameOpts(opts => {
      const newOpts = {
        ...opts,
        websocketOpts: data,
        roomSettings: !isDirectLink && !isSettings ? nextRoomSettings : opts.roomSettings,
        reset: false,
      };

      return isSettings ? {...newOpts, settingsChange: true} : newOpts;
    });

    if (setModalOpen) setModalOpen(false);

    if (!isSettings && !isDirectLink)
      gameOpts.setShowGame(true);
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
      setMaxPlayers(maxPlayerOpts[maxPlayerOpts.length - 1]);
      setIsSpectatorChecked(false);
    }
  }, [gameOpts.reset]);

  return (
    <div
      ref={newGameFormRef}
      className={isVisible ? styles.newGameForm : 'hidden'}
      onClick={(e) => e.stopPropagation()}
    >
      <form
        className={literata.className}
        onSubmit={handleSubmit}
        style={isDirectLink && { minWidth: 0 }}
      >
        <RequiredFields
          {...{
            goHome,
            isSettings, isDirectLink, isAdmin, isSpectatorChecked, handleSubmit,
            roomName, name, passwordLabel, tablePwd, tableLock, maxPlayers, tablePwdRef,
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
