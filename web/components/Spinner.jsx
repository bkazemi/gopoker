import Image from 'next/image';
import { Literata } from 'next/font/google';

import styles from '@/styles/Spinner.module.css';

const literata = Literata({ subsets: ['latin'], weight: '500' });

export default function Spinner({ msg }) {
  return (
    <div className={styles.spinner}>
      { msg && <p className={literata.className}>{msg}</p> }
      <Image
        src='/pokerchip3.png'
        width={100} height={100}
        alt='spinner'
      />
    </div>
  );
}
