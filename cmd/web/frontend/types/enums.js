export const CanvasWorkerMessageType = /** @type {const} */ ({
  Init: 0,
  CanvasDrag: 1,
  CellSizeChange: 2,
  Command: 3,
  Resize: 4,
  SetCells: 5,
  SetPattern: 6,
  SetSpeed: 7,
  SettingsChange: 8
});

export const Command = /** @type {const} */ ({
  Next: 0,
  Play: 1,
  Pause: 2,
  Clear: 3,
  Randomise: 4,
});

export const CanvasWorkerEventType = /** @type {const} */ ({
  Ready: 0,
  PlaybackStateChanged: 1,
  SpeedChanged: 2,
  Disconnected: 3,
});

export const EngineWorkerMessageType = /** @type {const} */ ({
  Init: 0,
  Snapshot: 1,
});

export const EngineWorkerEventType = /** @type {const} */ ({
  Ready: 0,
  Snapshot: 1,
});
