import { useSyncExternalStore } from 'react';

const serverSnapshot = {
  innerWidth: null,
};

let cachedSnapshot = serverSnapshot;

const getSnapshot = () => {
  if (typeof window === 'undefined')
    return serverSnapshot;

  const nextSnapshot = {
    innerWidth: window.innerWidth,
  };

  if (cachedSnapshot.innerWidth === nextSnapshot.innerWidth) {
    return cachedSnapshot;
  }

  cachedSnapshot = nextSnapshot;
  return cachedSnapshot;
};

const subscribe = (onStoreChange) => {
  if (typeof window === 'undefined')
    return () => {};

  window.addEventListener('resize', onStoreChange);
  return () => {
    window.removeEventListener('resize', onStoreChange);
  };
};

export default function useWindowMetrics() {
  return useSyncExternalStore(subscribe, getSnapshot, () => serverSnapshot);
}
