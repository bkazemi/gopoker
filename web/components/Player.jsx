import React, {useCallback, useContext, useEffect, useRef, useState} from 'react';

import Image from 'next/image';

import cx from 'classnames';
import { cloneDeep } from 'lodash';

import { TABLE_STATE, NETDATA, NetData, PlayerActionToString } from '@/lib/libgopoker';

import styles from '@/styles/Player.module.css';

const YourPlayerActions = ({ isYourPlayer, client, keyPressed, socket }) => {
  const betInputRef = useRef(null);
  const checkBtnRef = useRef(null);
  const callBtnRef = useRef(null);
  const raiseBtnRef = useRef(null);
  const foldBtnRef = useRef(null);
  const allInBtnRef = useRef(null);

  const [raiseInputValue, setRaiseInputValue] = useState('');
  const [raiseAmount, setRaiseAmount] = useState(BigInt(0));

  const btnRefMap = new Map([
    ['b', betInputRef],
    ['C', callBtnRef],
    ['c', checkBtnRef],
    ['r', raiseBtnRef],
    ['f', foldBtnRef],
    ['a', allInBtnRef],
  ]);

  const handleRaiseInput = useCallback((e) => {
    if (e.target.value === '') {
      if (raiseInputValue !== '') setRaiseInputValue('');
      if (raiseAmount !== 0n) setRaiseAmount(0n);
      return;
    }

    let multiplier = 1n;
    if (e.target.value.charAt(e.target.value.length - 1).match(/[Kk]/))
      multiplier = 1000n;
    if (e.target.value.charAt(e.target.value.length - 1).match(/[Mm]/))
      multiplier = 1000000n;

    const num = BigInt(e.target.value.replace(/[^0-9]/g, ''));
    let numStr = (num * multiplier).toLocaleString() || '';
    if (numStr === '0')
      numStr = '';

    setRaiseInputValue(numStr);
    setRaiseAmount(num * multiplier);
  }, [raiseInputValue, raiseAmount]);

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

    console.log(`sending ${btn} ${action}`);

    const newClient = {...client};
    newClient.Player = cloneDeep(client.Player);

    newClient.Player.Action.Action = action;
    newClient.Player.Action.Amount = raiseAmount;

    if (raiseAmount) {
      setRaiseInputValue('');
      setRaiseAmount(0);
    }

    socket.send(
      (new NetData(newClient, action)).toMsgPack()
    );
  }, [client, socket, raiseAmount]);

  // keyboard shortcuts
  useEffect(() => {
    const focusedKey = btnRefMap.get(keyPressed);
    if (focusedKey?.current) {
      console.log('focusedKey', focusedKey);
      if (focusedKey.current !== document.activeElement)
        focusedKey.current.focus();
    }
  }, [keyPressed]);

  if (!isYourPlayer)
    return;

  return (
    <div className={styles.yourPlayerActions}>
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
      <div
        className={cx(styles.yourPlayerActions,
          styles.buttons
      )}>
        <button ref={checkBtnRef} onClick={() => handleButton('check')}>check</button>
        <button ref={callBtnRef}  onClick={() => handleButton('call')}>call</button>
        <button ref={raiseBtnRef} onClick={() => handleButton('raise')}>raise</button>
        <button ref={foldBtnRef}  onClick={() => handleButton('fold')}>fold</button>
        <button ref={allInBtnRef} onClick={() => handleButton('allin')}>allin</button>
      </div>
    </div>
  );
};

const Positions = ({ tableState, isDealer, isSmallBlind, isBigBlind }) => {
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
};

export default function Player({
  key,
  client, socket, tableState, curPlayer,
  playerHead, dealerAndBlinds, side, gridRow, gridCol, isYourPlayer, keyPressed
}) {
  const [name, setName] = useState(client.Name);
  const [curAction, setCurAction] = useState(client.Player?.Action || {});
  const [chipCount, setChipCount] = useState(BigInt(client.Player?.ChipCount));

  const [isDealer, setIsDealer] = useState(false);
  const [isSmallBlind, setIsSmallBlind] = useState(false);
  const [isBigBlind, setIsBigBlind] = useState(false);

  const posSetStateMap = {
    dealer: setIsDealer,
    smallBlind: setIsSmallBlind,
    bigBlind: setIsBigBlind,
  };

  const [style, setStyle] = useState({gridRow, gridCol});

  useEffect(() => {
    setName(client.Name);
    setCurAction(client.Player?.Action);
    setChipCount(Number(client.Player?.ChipCount));
  }, [client, client.Player]);

  useEffect(() => {
    if (!client?.ID) {
      return;
    }

    console.log(`Players: cid: ${client.ID} curP: ${curPlayer?.ID} pHead: ${playerHead?.ID}`);

    if (client.ID === curPlayer?.ID)
      setStyle(s => {
        return {
          ...s,
          borderColor: 'red',
          borderWidth: '2px',
        }
      });
    else if (client.ID === playerHead?.ID)
      setStyle(s => {
        return {
          ...s,
          borderColor: '#eaa21f',
          borderWidth: '1px',
        }
      });
    else if (style.borderColor !== undefined && style.borderColor !== 'black')
      setStyle(s => {
        return {
          ...s,
          borderColor: 'black',
          borderWidth: '1px',
        }
      });
  }, [client, curPlayer, playerHead]);

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
  }, [dealerAndBlinds, client]);

  if (client._ID) {
    return (
      <div
        key={String(Math.random())}
        className={styles.player}
        style={style}
      >
        <div className={styles.nameContainer}>
          <p className={styles.name}>{name}{isYourPlayer && <span style={{fontStyle: 'italic'}}> (You)</span>}</p>
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
      key={key}
      className={styles.player}
      style={style}
    >
      <div className={styles.nameContainer}>
        <p className={styles.name}>{name}{isYourPlayer && <span style={{fontStyle: 'italic'}}> (You)</span>}</p>
        <Positions {...{tableState, isDealer, isSmallBlind, isBigBlind}} />
      </div>
      <p>current action: { PlayerActionToString(curAction) }</p>
      <p>chip count: { chipCount.toLocaleString() }</p>
      <YourPlayerActions {...{isYourPlayer, client, keyPressed, socket}} />
    </div>
  );
}
