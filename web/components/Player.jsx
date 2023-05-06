import React, {useCallback, useContext, useEffect, useRef, useState} from 'react';

import styles from '@/styles/Player.module.css';

import cx from 'classnames';

import { NETDATA, NetData, PlayerActionToString } from '@/lib/libgopoker';

const YourPlayerActions = ({ isYourPlayer, client, keyPressed, socket }) => {
  if (!isYourPlayer)
    return; 

  const checkBtnRef = useRef(null);
  const callBtnRef = useRef(null);
  const raiseBtnRef = useRef(null);
  const foldBtnRef = useRef(null);
  const allInBtnRef = useRef(null);

  const [raiseAmount, setRaiseAmount] = useState(BigInt(0));

  const btnRefMap = new Map([
    ['C', callBtnRef],
    ['c', checkBtnRef],
    ['r', raiseBtnRef],
    ['f', foldBtnRef],
    ['a', allInBtnRef],
  ]);

  const handleRaiseInput = (e) => {
    let multiplier = 1n;
    if (e.target.value.charAt(e.target.value.length - 1).match(/[Kk]/))
      multiplier = 1000n;
    if (e.target.value.charAt(e.target.value.length - 1).match(/[Mm]/))
      multiplier = 1000000n;

    const num = BigInt(e.target.value.replace(/[^0-9]/g, ''));
    e.target.value = (num * multiplier).toLocaleString() || '';
    if (e.target.value === '0')
      e.target.value = '';
    setRaiseAmount(num * multiplier);
  };

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

    client.Player.Action.Action = action;
    client.Player.Action.Amount = raiseAmount;

    socket.send(
      (new NetData(client, action)).toMsgPack()
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

  return (
    <div className={styles.yourPlayerActions}>
      <label htmlFor='raiseInput'>bet amount</label>
      <input
        id='raiseInput'
        type='text'
        name='raiseInput'
        step={10}
        min={0}
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

export default function Player({ client, socket, curPlayer, playerHead, side,
  gridRow, gridCol, isYourPlayer, keyPressed }) {
  const [name, setName] = useState(client.Name);
  const [curAction, setCurAction] = useState(client.Player?.Action || {});
  const [chipCount, setChipCount] = useState(Number(client.Player?.ChipCount));

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

  return (
    <div key={client?.ID || String(Math.random())} className={styles.player} style={style}>
      <p className={styles.name}>{name}{isYourPlayer && <span style={{fontStyle: 'italic'}}> (You)</span>}</p>
      <p>current action: {PlayerActionToString(curAction)}</p>
      <p>chip count: {chipCount.toLocaleString()}</p>
      <YourPlayerActions {...{isYourPlayer, client, keyPressed, socket}} />
    </div>
  );
}
