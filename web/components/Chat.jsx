import React, {useState, useCallback, useEffect} from 'react';

import { NETDATA, NetData } from '@/lib/libgopoker';

import styles from '@/styles/Chat.module.css';

export default function Chat({ socket, yourClient, msgs }) {
  const [msg, setMsg] = useState(''); 
  const [netData, setNetData] = useState(new NetData(yourClient, NETDATA.CHAT_MSG, msg));
  const [chatMsgsStyle, setChatMsgsStyle] = useState({borderColor: 'black', borderWidth: '1px'});

  const sendMsg = useCallback(() => {
    if (msg) {
      console.warn(`sendMsg: ${msg}`);
      socket.send(
        netData.toMsgPack()
      );
      setMsg('');
    }
  }, [socket, msg, netData]);

  const handleChatMsgsMouseEnter = () => {
    setChatMsgsStyle({ borderColor: 'black', borderWidth: '1px' });
  };

  useEffect(() => {
    yourClient && msg && setNetData(netData => {
      return new NetData(yourClient, NETDATA.CHAT_MSG, msg);
    });
  }, [yourClient, msg]);

  useEffect(() => {
    if (msgs.length && chatMsgsStyle.borderColor === 'black')
      setChatMsgsStyle({borderColor: 'green', borderWidth: '2px'});
  }, [msgs]);

  return (
    <div className={styles.chatContainer}>
      <label className={styles.chatLabel}>chat</label>
      <div className={styles.chatMsgs} style={chatMsgsStyle} onMouseEnter={handleChatMsgsMouseEnter}>
        {
          msgs
            .map((msg, idx) => {
              return (
                <div key={ idx } style={{
                  fontWeight: msg.startsWith('<server-msg>') ? 'bold' : 'normal',
                  whiteSpace: 'pre-wrap',
                }}>
                { msg }
              </div>);
            })
        }
      </div>
      <div className={ styles.chatSendMsgContainer }>
        <textarea
          className={ styles.chatInput }
          onChange={ e => setMsg(e.target.value) }
          onSubmit={ sendMsg }
          value={ msg }
          placeholder='your chat message'
        />
        <button className={ styles.chatSendBtn } onClick={ sendMsg }>send</button>
      </div>
    </div>
  );
}
