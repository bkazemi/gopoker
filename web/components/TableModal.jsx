import React, { useState, useEffect, useContext } from 'react';

import Modal from 'react-modal';

import Image from 'next/image';
import { useRouter } from 'next/router';

import NewGameForm from '@/components/NewGameForm';

import styles from '@/styles/TableModal.module.css';

import { Literata } from 'next/font/google';

const literata = Literata({ subsets: ['latin', 'latin-ext'], weight: '500' });

import cx from 'classnames';
import { GameContext } from '@/GameContext';
import { NewClient } from '@/lib/libgopoker';

const ModalContent = ({ modalType, modalTxt, modalOpen, setModalOpen, setShowGame, setFormData, setGameOpts }) => {
  const router = useRouter();

  const [pageIdx, setPageIdx] = useState(0);

  const goHome = () => {
    setGameOpts(opts => ({
      ...opts,
      client: NewClient({}),
      roomURL: '', reset: true,
    }));
    router.push('/');
  };

  switch (modalType) {
  case 'preGame':
    return (
      <>
        <p className={styles.modalTxt}>{ modalTxt[pageIdx] }</p>
        <button
          className={styles.modalBtn}
          onClick={goHome}
        >
          go home
        </button>
      </>
    );
  case 'quit':
    return (
      <>
       <div
          style={{
            display: 'flex',
            alignSelf: 'flex-start',
            paddingLeft: '4.5rem',
            paddingBottom: '7px',
          }}
        >
          <Image
            src={'/quitGame.png'}
            height={35}
            width={35}
            alt={'<quitGame image>'}
            style={{
              alignSelf: 'flex-start',
              marginRight: '10px',
            }}
          />
          <h2>quit game</h2>
        </div>
        <p className={styles.modalTxt}>{ modalTxt[pageIdx] }</p>
        <div
          style={{
            display: 'flex',
            gap: '3px',
            paddingTop: '7px',
          }}
        >
          <button
            className={styles.modalBtn}
            style={{ marginRight: '3px' }}
            onClick={goHome}
          >
            quit
          </button>
          <button
            className={styles.modalBtn}
            onClick={() => setModalOpen(false)}
          >
            cancel
          </button>
        </div>
      </>
    );
  case 'settings':
    return (
      <>
        <div
          style={{
            display: 'flex',
            alignSelf: 'flex-start',
            paddingLeft: '2rem',
            paddingBottom: '7px',
          }}
        >
          <Image
            src={'/settingsIcon.png'}
            height={35}
            width={35}
            alt={'<settings image>'}
            style={{
              alignSelf: 'flex-start',
              marginRight: '10px',
            }}
          />
          <h2>settings</h2>
        </div>
        <NewGameForm
          isVisible={true}
          isSettings={true}
          setModalOpen={setModalOpen}
          setFormData={setFormData}
        />
      </>
    );
  default:
    if (!modalTxt.length) {
      setModalOpen(false);
      return;
    }

    return (
      <>
        <p className={styles.modalTxt}>{ modalTxt[pageIdx] }</p>
        <div
          style={{
            display: 'flex',
            gap: '3px',
            paddingTop: '7px',
          }}
        >
          {
            modalTxt.length > 1 &&
            <button
              className={styles.modalBtn}
              onClick={() => setPageIdx(idx => (idx + 1) % modalTxt.length)}
            >
              { pageIdx === (modalTxt.length - 1) ? 'first page' : 'next page' }
            </button>
          }
          <button
            className={styles.modalBtn}
            onClick={() => setModalOpen(false)}
          >
            close
          </button>
      </div>
      </>
    );
  }
};

export default function TableModal({
  modalType, modalTxt, setModalTxt,
  modalOpen, setModalOpen, setShowGame, setFormData
}) {
  const {gameOpts, setGameOpts} = useContext(GameContext);

  useEffect(() => {
    if (!modalOpen)
      setModalTxt([]);
  }, [modalOpen]);

  return (
    <Modal
      ariaHideApp={false}
      isOpen={modalOpen}
      onRequestClose={() => setModalOpen(false)}
      shouldCloseOnOverlayClick={modalType !== 'preGame'}
      shouldCloseOnEsc={modalType !== 'preGame'}
      contentLabel='label'
      style={{
        overlay: {
          backgroundColor: modalType === 'preGame' ? '#d7d7d7' : 'transparent',
          zIndex: 2,
        },
        content: {
          top: '50%',
          left: '50%',
          right: 'auto',
          bottom: 'auto',
          marginRight: '-50%',
          transform: 'translate(-50%, -50%)',
          minWidth: '350px', minHeight: '350px',
          borderRadius: '10px',
          border: '5px double',
          zIndex: 2,
          overflow: 'auto',
        },
      }}
    >
      <div
        className={cx(
          styles.contentFlex,
          literata.className
        )}
      >
        <ModalContent
          {...{modalType, modalTxt, modalOpen, setModalOpen, setShowGame,
               setFormData, setGameOpts}}
        />
      </div>
    </Modal>
  );
}
