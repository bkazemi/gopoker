import { useState, useSyncExternalStore } from 'react';

const serverSnapshot = {
  innerWidth: null,
  screenWidth: null,
};

const createInitialMetricsStore = () => {
  let initialSnapshot = serverSnapshot;
  let cachedSnapshot = serverSnapshot;

  const getLiveSnapshot = () => {
    if (typeof window === 'undefined')
      return serverSnapshot;

    const nextSnapshot = {
      innerWidth: window.innerWidth,
      screenWidth: window.screen?.width ?? window.innerWidth,
    };

    if (
      cachedSnapshot.innerWidth === nextSnapshot.innerWidth
      && cachedSnapshot.screenWidth === nextSnapshot.screenWidth
    ) {
      return cachedSnapshot;
    }

    cachedSnapshot = nextSnapshot;
    return cachedSnapshot;
  };

  return {
    subscribe(onStoreChange) {
      if (typeof window === 'undefined')
        return () => {};

      window.addEventListener('resize', onStoreChange);
      return () => {
        window.removeEventListener('resize', onStoreChange);
      };
    },
    getSnapshot() {
      const liveSnapshot = getLiveSnapshot();

      if (initialSnapshot === serverSnapshot && liveSnapshot.innerWidth !== null)
        initialSnapshot = liveSnapshot;

      return initialSnapshot === serverSnapshot ? liveSnapshot : initialSnapshot;
    },
    getServerSnapshot() {
      return serverSnapshot;
    },
  };
};

export default function useInitialWindowMetrics() {
  const [store] = useState(createInitialMetricsStore);
  return useSyncExternalStore(store.subscribe, store.getSnapshot, store.getServerSnapshot);
}
