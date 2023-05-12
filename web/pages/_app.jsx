import React, { useEffect } from 'react';

import '@/styles/globals.css'

export default function App({ Component, pageProps }) {
  // confirm window exit
  useEffect(() => {
    const handleBeforeUnload = (e) => {
      e.preventDefault();
      e.returnValue = '';

      return '';
    };

    window.addEventListener('beforeunload', handleBeforeUnload);

    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload)
    }
  }, []);

  return <Component {...pageProps} />
}
