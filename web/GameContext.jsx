import React, { createContext, useState } from 'react';

export const GameContext = createContext();

export const GameProvider = ({ children }) => {
  const [gameOpts, setGameOpts] = useState({});

  return (
    <GameContext.Provider value={{ gameOpts, setGameOpts }}>
      {children}
    </GameContext.Provider>
  );
};
