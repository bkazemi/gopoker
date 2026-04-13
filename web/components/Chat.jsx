import React, {useState, useCallback, useEffect, useRef} from 'react';

import { VT323 } from 'next/font/google';

import cx from 'classnames';

import { NETDATA, NetData } from '@/lib/libgopoker';

import styles from '@/styles/Chat.module.css';

const vt323 = VT323({ subsets: ['latin', 'latin-ext', 'vietnamese'], weight: '400' });

const formattedMsg = (msg) => {
  if (!msg.startsWith('{ '))
    return msg;

  return (
    <>
      <span style={{ fontStyle: 'italic' }}>{'{noname '}</span>
      { msg.substring(2) }
    </>
  );
};

function Chat({ socket, yourClient, msgs, chatInputRef }) {
  const chatMsgsRef = useRef(null);

  const [msg, setMsg] = useState('');
  const [lastReadMsgCount, setLastReadMsgCount] = useState(msgs.length);
  const [chatMsgsUserScrolled, setChatMsgsUserScrolled] = useState(false);
  const chatMsgsAtBottomRef = useRef(true);
  const scrollTimerRef = useRef(null);
  const hasUnreadMsgs = msgs.length > lastReadMsgCount;

  const sendMsg = useCallback(() => {
    if (msg && yourClient) {
      console.warn(`sendMsg: ${msg}`);
      socket.send(new NetData(yourClient, NETDATA.CHAT_MSG, msg).toMsgPack());
      setMsg('');
    }
  }, [socket, yourClient, msg]);

  const handleChatMsgsMouseEnter = useCallback(() => {
    setLastReadMsgCount(msgs.length);
  }, [msgs.length]);

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
    if (msgs.length) {
      if (chatMsgsAtBottomRef.current)
        scrollTimerRef.current = setTimeout(scrollToBottomOfChatMsgs, 100);
    }

    return () => {
      if (scrollTimerRef.current) {
        clearTimeout(scrollTimerRef.current);
        scrollTimerRef.current = null;
      }
    };
  }, [msgs, scrollToBottomOfChatMsgs]);

  return (
    <div className={styles.chatContainer}>
      <label className={styles.chatLabel}>chat</label>
      <div
        ref={chatMsgsRef}
        className={cx(styles.chatMsgs, vt323.className)}
        style={{
          borderColor: hasUnreadMsgs ? 'green' : 'black',
          borderWidth: hasUnreadMsgs ? '2px' : '1px',
        }}
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
                  { formattedMsg(msg) }
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
