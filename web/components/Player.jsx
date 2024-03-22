import React, {useCallback, useEffect, useRef, useState, useMemo} from 'react';

import Image from 'next/image';
import { Literata } from 'next/font/google';

import cx from 'classnames';
import { cloneDeep } from 'lodash';
import { Tooltip } from 'react-tooltip';

import { TABLE_STATE, NETDATA, NetData, NetDataToPlayerState, PlayerStateToString, PLAYERSTATE } from '@/lib/libgopoker';

import styles from '@/styles/Player.module.css';

const literata = Literata({ subsets: ['latin'], weight: '500' });

const YourPlayerActions = React.memo(({
  isYourPlayer, isSmallBlind, curPlayer, tableState, client, keyPressed,
  socket,
}) => {
  const betInputRef = useRef(null);
  const checkBtnRef = useRef(null);
  const callBtnRef = useRef(null);
  const raiseBtnRef = useRef(null);
  const foldBtnRef = useRef(null);
  const allInBtnRef = useRef(null);

  const [raiseInputValue, setRaiseInputValue] = useState('');
  const [raiseAmount, setRaiseAmount] = useState(BigInt(0));

  const [isCheckDisabled, setIsCheckDisabled] = useState(false);
  const [isCallDisabled, setIsCallDisabled] = useState(false);
  const [isRaiseDisabled, setIsRaiseDisabled] = useState(false);
  const [isFoldDisabled, setIsFoldDisabled] = useState(false);
  const [isAllinDisabled, setIsAllinDisabled] = useState(false);

  const btnRefMap = useMemo(() => (
      new Map([
        ['b', betInputRef],
        ['C', callBtnRef],
        ['c', checkBtnRef],
        ['r', raiseBtnRef],
        ['f', foldBtnRef],
        ['a', allInBtnRef],
      ])
    ), []);

  const btnCursorStyle = useCallback((isBtnDisabled) => {
    return { cursor: isBtnDisabled ? 'default' : 'pointer' };
  }, []);

  const handleRaiseInput = useCallback((e) => {
    if (e.target.value === '') {
      if (raiseInputValue !== '') setRaiseInputValue('');
      if (raiseAmount !== 0n) setRaiseAmount(0n);
      return;
    } else if (e.target.value === '+') {
      if (raiseInputValue !== '+') setRaiseInputValue('+');
      if (raiseAmount !== 0n) setRaiseAmount(0n);
      return;
    }

    let betBase = 0n;
    let betBaseChar = '';
    if (e.target.value.charAt(0) === '+') {
      betBase = client.Player.Action.Amount;
      betBaseChar = '+';
    }

    let multiplier = 1n;
    if (e.target.value.charAt(e.target.value.length - 1).match(/[Kk]/))
      multiplier = 1000n;
    if (e.target.value.charAt(e.target.value.length - 1).match(/[Mm]/))
      multiplier = 1000000n;

    const num = BigInt(e.target.value.replace(/[^0-9]/g, ''));
    let numStr = betBaseChar + ((num * multiplier).toLocaleString() || '');
    if (numStr === '0')
      numStr = '';
    else if (numStr === '+0')
      numStr = '+';

    setRaiseInputValue(numStr);
    setRaiseAmount(betBase + (num * multiplier));
  }, [client, raiseInputValue, raiseAmount]);

  const handleButton = useCallback((btn) => {
    const map = new Map([
      ['call',  NETDATA.CALL],
      ['check', NETDATA.CHECK],
      ['raise', NETDATA.BET],
      ['fold',  NETDATA.FOLD],
      ['allin', NETDATA.ALLIN],
    ]);

    const action = map.get(btn);
    if (!action) {
      console.error(`Player: handleButton: BUG: ${btn} not in map`);
      return;
    }

    if (btn === 'raise' && !raiseAmount)
      return; // pressed raise btn with 0 chips/no bet input

    console.log(`sending ${btn} ${action}`);

    const newClient = {...client};
    newClient.Player = cloneDeep(client.Player);

    newClient.Player.Action.Action = NetDataToPlayerState(action);
    newClient.Player.Action.Amount = raiseAmount;

    if (raiseAmount) {
      setRaiseInputValue('');
      setRaiseAmount(0);
    }

    socket.send(
      (new NetData(newClient, action)).toMsgPack()
    );
  }, [client, socket, raiseAmount]);

  // enable/disable action buttons as appropriate
  useEffect(() => {
    const notStartedOrYourTurn =
      (tableState === TABLE_STATE.NOT_STARTED || curPlayer?.ID !== client.ID);
    const isSmallBlindPreflop =
      (isSmallBlind && tableState === TABLE_STATE.PREFLOP);
    const playerRaised = tableState === TABLE_STATE.PLAYER_RAISED;
    const isAllIn = client.Player?.Action.Action === PLAYERSTATE.ALLIN;

    setIsCheckDisabled(notStartedOrYourTurn || isAllIn || isSmallBlindPreflop || playerRaised);
    setIsCallDisabled(notStartedOrYourTurn  || isAllIn || (!isSmallBlindPreflop && !playerRaised));
    setIsRaiseDisabled(notStartedOrYourTurn || isAllIn)
    setIsFoldDisabled(notStartedOrYourTurn  || isAllIn)
    setIsAllinDisabled(notStartedOrYourTurn || isAllIn);
  }, [client, curPlayer, tableState, isSmallBlind]);

  // keyboard shortcuts
  useEffect(() => {
    const focusedKey = btnRefMap.get(keyPressed);
    if (focusedKey?.current) {
      console.log('focusedKey', focusedKey);
      if (focusedKey.current !== document.activeElement)
        focusedKey.current.focus();
    }
  }, [keyPressed, btnRefMap]);

  if (!isYourPlayer)
    return;

  return (
    <div className={styles.yourPlayerActions}>
      <div className={styles.betContainer}>
        <label htmlFor='raiseInput'>bet amount</label>
        <input
          ref={betInputRef}
          id='raiseInput'
          type='text'
          name='raiseInput'
          step={10}
          min={0}
          value={raiseInputValue}
          onInput={handleRaiseInput}
          /*max={} TODO: add me */
        />
        <a style={{ display: 'flex' }} data-tooltip-id="betTooltip">
          <Image
            src={'/betHelp.png'}
            width={29}
            height={29}
            alt={'<bet help img>'}
          />
        </a>
        <Tooltip id="betTooltip" style={{ zIndex: 5 }}>
          <pre className={styles.betTooltipTxt}>
            {`The amount to bet.

             Typing K or k after a number multiplies the bet by 1,000.
             Typing M or m after a number multiplies the bet by 1,000,000.
             Typing '+' before a number sets your bet amount to your last bet + <number>

             Examples:

             100k => 100,000
             5m => 5,000,000

             last bet: 250 chips
             +100 => 250+100 => 350
            `.replace(/^ +/gm, '')}
          </pre>
        </Tooltip>
      </div>
      <div
        className={cx(styles.yourPlayerActions,
          styles.buttons, literata.className
      )}>
        <button
          ref={checkBtnRef}
          disabled={isCheckDisabled}
          style={{ ...btnCursorStyle(isCheckDisabled), }}
          onClick={() => handleButton('check')}
        >
          check
        </button>
        <button
          ref={callBtnRef}
          disabled={isCallDisabled}
          style={{ ...btnCursorStyle(isCallDisabled), }}
          onClick={() => handleButton('call')}
        >
          call
        </button>
        <button
          ref={raiseBtnRef}
          disabled={isRaiseDisabled}
          style={{ ...btnCursorStyle(isRaiseDisabled), }}
          onClick={() => handleButton('raise')}
        >
          raise
        </button>
        <button
          ref={foldBtnRef}
          disabled={isFoldDisabled}
          style={{ ...btnCursorStyle(isFoldDisabled), }}
          onClick={() => handleButton('fold')}
        >
          fold
        </button>
        <button
          ref={allInBtnRef}
          disabled={isAllinDisabled}
          style={{ ...btnCursorStyle(isAllinDisabled), }}
          onClick={() => handleButton('allin')}
        >
          allin
        </button>
      </div>
    </div>
  );
});

YourPlayerActions.displayName = 'YourPlayerActions';

const Positions = React.memo(({ tableState, isDealer, isSmallBlind, isBigBlind }) => {
  if (tableState === TABLE_STATE.NOT_STARTED ||
     (!isDealer && !isBigBlind && !isSmallBlind))
    return;

  return (
    <div className={styles.positions}>
      {
        isDealer &&
        <Image
          src={'/D.png'}
          width={35}
          height={35}
          alt='[D]'
        />
      }
      {
        isSmallBlind &&
        <Image
          src={'/SB.png'}
          width={35}
          height={35}
          alt='[Sb]'
        />
      }
      {
        isBigBlind &&
        <Image
          src={'/BB.png'}
          width={35}
          height={35}
          alt='[Bb]'
        />
      }
    </div>
  );
});

Positions.displayName = 'Positions';

const ReconnectOverlay = React.memo(() => (
  <div className={styles.reconnectOverlay}>
    <Image
      src={'/connectionLost.png'}
      width={45}
      height={45}
      alt='[connectionLost img]'
    />
    <p>waiting for reconnect...</p>
  </div>
));

ReconnectOverlay.displayName = 'ReconnectOverlay';

function Player({
  client, socket, tableState, curPlayer,
  playerHead, dealerAndBlinds, side, gridRow, gridCol, isYourPlayer, keyPressed
}) {
  const [name, setName] = useState(client.Name);
  const [curAction, setCurAction] = useState(client.Player?.Action || {});
  const [chipCount, setChipCount] = useState(BigInt(client.Player?.ChipCount));

  const [isDealer, setIsDealer] = useState(false);
  const [isSmallBlind, setIsSmallBlind] = useState(false);
  const [isBigBlind, setIsBigBlind] = useState(false);

  const [isReconnecting, setIsReconnecting] = useState(client.Player?.isDisconnected);

  const posSetStateMap = useMemo(() => ({
    dealer:     setIsDealer,
    smallBlind: setIsSmallBlind,
    bigBlind:   setIsBigBlind,
  }), []);

  const [style, setStyle] = useState({gridRow, gridCol});

  useEffect(() => {
    setName(client.Name);
    setCurAction(client.Player?.Action);
    setChipCount(Number(client.Player?.ChipCount));
    setIsReconnecting(client.Player?.isDisconnected);
  }, [client, client.Player]);

  useEffect(() => {
    if (!client?.ID) {
      if (style.borderColor !== undefined && style.borderColor !== 'black')
        setStyle(s => ({
          ...s,
          borderColor: 'black',
          borderWidth: '1px',
        }));

      return;
    }

    console.log(`Players: cid: ${client.ID} curP: ${curPlayer?.ID} pHead: ${playerHead?.ID}`);

    if (client.ID === curPlayer?.ID)
      setStyle(s => ({
          ...s,
          borderColor: 'red',
          borderWidth: '2px',
      }));
    else if (client.ID === playerHead?.ID)
      setStyle(s => ({
          ...s,
          borderColor: '#eaa21f',
          borderWidth: '1px',
      }));
    else if (style.borderColor !== undefined && style.borderColor !== 'black')
      setStyle(s => ({
          ...s,
          borderColor: 'black',
          borderWidth: '1px',
      }));
  }, [client, curPlayer, playerHead, style.borderColor]);

  useEffect(() => {
    setStyle(s => ({
      ...s,
      position: isReconnecting ? 'relative' : ''
    }));
  }, [isReconnecting]);

  useEffect(() => {
    if (!side)
      return;

    setStyle(s => {
      const sty = {...s};
      if (side === 'left') {
        sty.borderRight = 0;
        sty.borderTopRightRadius = 0;
        sty.borderBottomRightRadius = 0;
      } else if (side === 'right') {
        sty.borderLeft = 0;
        sty.borderTopLeftRadius = 0;
        sty.borderBottomLeftRadius = 0;
      } else if (side === 'top') {
        sty.borderBottom = 0;
        sty.borderBottomLeftRadius = 0;
        sty.borderBottomRightRadius = 0;
        sty.boxShadow = 'none';
      } else {
        sty.borderTop = 0;
        sty.borderTopLeftRadius = 0;
        sty.borderTopRightRadius = 0;
      }

      return sty;
    });
  }, [side]);

  useEffect(() => {
    [...Object.entries(dealerAndBlinds)]
      .filter(([name, seat]) => {
        //console.log(`dSB ${name} name: ${seat?.Player?.Name} cid ${client?._ID}  cname ${client?.Player?.Name}`);
        if (seat?.Player?.Name === client?.Player?.Name)
          return true;
        else
          posSetStateMap[name](false);
      })
      .map(([name]) => {
        //console.log(`MAP ${client.Player.Name} ${name}`);
        posSetStateMap[name](true);
      });
  }, [dealerAndBlinds, client, posSetStateMap]);

  if (client._ID) { // vacant seat
    return (
      <div
        key={String(Math.random())}
        className={styles.player}
        style={style}
      >
        <div className={styles.nameContainer}>
          <p className={styles.name}>{name}</p>
        </div>
        <Image
          src={'/seat.png'}
          height={85}
          width={60}
          style={{
            marginTop: '10px',
            paddingBottom: '5px',
            width: 'auto',
          }}
          alt='[seat img]'
        />
      </div>
    );
  }

  return (
    <div
      className={styles.player}
      style={style}
    >
      <div
        className={styles.nameContainer}
        style={{ filter: !isYourPlayer && isReconnecting ? 'blur(1.5px)' : '' }}
      >
        <p className={styles.name}>{name}{isYourPlayer && <span style={{fontStyle: 'italic'}}> (You)</span>}</p>
        <Positions {...{tableState, isDealer, isSmallBlind, isBigBlind}} />
      </div>
      <p
        style={{ filter: !isYourPlayer && isReconnecting ? 'blur(1.5px)' : '' }}
      >
        current action: { PlayerStateToString(curAction) }
      </p>
      <div
        className={styles.chipCountContainer}
        style={{ filter: !isYourPlayer && isReconnecting ? 'blur(1.5px)' : '' }}
      >
        <p>chip count: { chipCount.toLocaleString() }</p>
        <Image
          src={'/chipCountChips.png'}
          width={30}
          height={30}
          alt={'<chipCount img>'}
        />
      </div>
      <YourPlayerActions {...{isYourPlayer, curPlayer, isSmallBlind, tableState, client, keyPressed, socket}} />
      { !isYourPlayer && isReconnecting && <ReconnectOverlay /> }
    </div>
  );
}

Player.displayName = 'Player';

export default React.memo(Player);
