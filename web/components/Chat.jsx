import React, {useState, useCallback, useEffect, useRef} from 'react';

import cx from 'classnames';

import { VT323 } from 'next/font/google';

import { NETDATA, NetData } from '@/lib/libgopoker';

import styles from '@/styles/Chat.module.css';

const vt323 = VT323({ subsets: ['latin', 'latin-ext', 'vietnamese'], weight: '400' });

function Chat({ socket, yourClient, msgs, chatInputRef }) {
  const chatMsgsRef = useRef(null);

  const [msg, setMsg] = useState('');
  const [netData, setNetData] = useState(new NetData(yourClient, NETDATA.CHAT_MSG, msg));
  const [chatMsgsStyle, setChatMsgsStyle] = useState({borderColor: 'black', borderWidth: '1px'});
  const [chatMsgsUserScrolled, setChatMsgsUserScrolled] = useState(false);
  const chatMsgsAtBottomRef = useRef(true);

  const sendMsg = useCallback(() => {
    if (msg) {
      console.warn(`sendMsg: ${msg}`);
      socket.send(
        netData.toMsgPack()
      );
      setMsg('');
    }
  }, [socket, msg, netData]);

  const handleChatMsgsMouseEnter = useCallback(() => {
    setChatMsgsStyle({ borderColor: 'black', borderWidth: '1px' });
  }, [setChatMsgsStyle]);

  const scrollToBottomOfChatMsgs = useCallback(() => {
    if (chatMsgsRef.current)
      chatMsgsRef.current.scrollTop = chatMsgsRef.current.scrollHeight;
    if (!chatMsgsAtBottomRef.current)
      chatMsgsAtBottomRef.current = true;
  }, [chatMsgsRef, chatMsgsAtBottomRef]);

  const handleChatMsgsScroll = useCallback(() => {
    if (!chatMsgsUserScrolled) {
      return;
    }

    const chatMsgs = chatMsgsRef.current;

    const isScrollAtBottom =
      chatMsgs.scrollTop + chatMsgs.clientHeight >=
      chatMsgs.scrollHeight - 1;

    chatMsgsAtBottomRef.current = isScrollAtBottom;
    if (isScrollAtBottom) setChatMsgsUserScrolled(false);
  }, [chatMsgsUserScrolled]);

  const handleChatMsgsUserScroll = useCallback(() => {
    setChatMsgsUserScrolled(true);
  }, [setChatMsgsUserScrolled]);

  useEffect(() => {
    yourClient && msg && setNetData(_ => {
      return new NetData(yourClient, NETDATA.CHAT_MSG, msg);
    });
  }, [yourClient, msg]);

  useEffect(() => {
    if (msgs.length) {
      setChatMsgsStyle({ borderColor: 'green', borderWidth: '2px' });
      chatMsgsAtBottomRef.current && setTimeout(scrollToBottomOfChatMsgs, 100);
    }
  }, [msgs, scrollToBottomOfChatMsgs]);

  return (
    <div className={styles.chatContainer}>
      <label className={styles.chatLabel}>chat</label>
      <div
        ref={chatMsgsRef}
        className={cx(styles.chatMsgs, vt323.className)}
        style={chatMsgsStyle}
        onMouseEnter={handleChatMsgsMouseEnter}
        onScroll={handleChatMsgsScroll}
        onMouseDown={handleChatMsgsUserScroll}
        onTouchStart={handleChatMsgsUserScroll}
        onWheel={handleChatMsgsUserScroll}
      >
        {
          msgs
            .map((msg, idx) => {
              return (
                <div
                  key={idx}
                  style={{
                    fontWeight: msg.startsWith('<') ? 'bold' : 'normal',
                    whiteSpace: 'pre-wrap',
                  }}
                >
                  { msg }
                </div>
              );
            })
        }
      </div>
      <div className={styles.chatSendMsgContainer}>
        <textarea
          ref={chatInputRef}
          className={styles.chatInput}
          onChange={e => setMsg(e.target.value)}
          onSubmit={sendMsg}
          value={msg}
          placeholder='your chat message'
        />
        <button className={styles.chatSendBtn} onClick={sendMsg}>send</button>
      </div>
    </div>
  );
}

Chat.displayName = 'Chat';

export default React.memo(Chat);
