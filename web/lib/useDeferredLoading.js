import { useState, useEffect } from 'react';

export default function useDeferredLoading(isLoading, delay = 500) {
  const [showLoading, setShowLoading] = useState(false);

  useEffect(() => {
    if (!isLoading) {
      setShowLoading(false);
      return;
    }

    const timer = setTimeout(() => setShowLoading(true), delay);
    return () => clearTimeout(timer);
  }, [isLoading, delay]);

  return showLoading;
}
