import type { CanvasWorkerEventType, CanvasWorkerMessageType, Command, EngineWorkerEventType, EngineWorkerMessageType } from './enums';

// #region canvas worker message

export declare type CanvasWorkerInitMessage = {
  type: typeof CanvasWorkerMessageType.Init;
  canvas: OffscreenCanvas;
  cellSize: number;
  height: number;
  width: number;
  worldSize: number;
  wsPath: string; // empty for a single-player game
  enginePort?: MessagePort; // set for a single-player game: the port to the engine worker that runs it
};

export declare type CanvasDragMessage = {
  type: typeof CanvasWorkerMessageType.CanvasDrag;
  dx: number;
  dy: number;
};

export declare type CellSizeChangeMessage = {
  type: typeof CanvasWorkerMessageType.CellSizeChange;
  cellSize: number;
  mouseX: number;
  mouseY: number;
};

export declare type CommandMessage = {
  type: typeof CanvasWorkerMessageType.Command;
  cmd: typeof Command[keyof typeof Command];
};

export declare type ResizeMessage = {
  type: typeof CanvasWorkerMessageType.Resize;
  height: number;
  width: number;
};

export declare type SetCellsMessage = {
  type: typeof CanvasWorkerMessageType.SetCells;
  count: number;
  colour: number;
  coordinates: Uint8Array; // x | y
  originPx: number;
  originPy: number;
};

export declare type SetPatternMessage = {
  type: typeof CanvasWorkerMessageType.SetPattern;
  colour: number;
  patternType: string;
  originPx: number;
  originPy: number;
};

export declare type SetSpeedMessage = {
  type: typeof CanvasWorkerMessageType.SetSpeed;
  speed: number;
};

export declare type SettingsChangeMessage = {
  type: typeof CanvasWorkerMessageType.SettingsChange;
  drawAge: boolean;
  drawGrid: boolean;
  backgroundColour: number;
  gridColour: number;
};

export declare type CanvasWorkerMessage = CanvasWorkerInitMessage
  | CanvasDragMessage
  | CellSizeChangeMessage
  | CommandMessage
  | ResizeMessage
  | SetCellsMessage
  | SetPatternMessage
  | SetSpeedMessage
  | SettingsChangeMessage;

// #endregion canvas worker message

// #region canvas worker event

export declare type ReadyEvent = {
  type: typeof CanvasWorkerEventType.Ready;
};

export declare type PlaybackStateChangedEvent = {
  type: typeof CanvasWorkerEventType.PlaybackStateChanged;
  playing: boolean;
};

export declare type SpeedChangedEvent = {
  type: typeof CanvasWorkerEventType.SpeedChanged;
  speed: number;
};

export declare type DisconnectedEvent = {
  type: typeof CanvasWorkerEventType.Disconnected;
  code: number;
  reason: string;
};

export declare type CanvasWorkerEvent = PlaybackStateChangedEvent | ReadyEvent | SpeedChangedEvent | DisconnectedEvent;

// #endregion canvas worker event

// #region engine worker (cmd/wasmengine), which runs a single-player game

export declare type EngineWorkerInitMessage = {
  type: typeof EngineWorkerMessageType.Init;
  worldSize: number;
  seed: Uint8Array; // the saved state, an encoded protocol.Output; empty for a new game
  port: MessagePort; // the canvas worker plays the game on the other end, with the wire protocol
};

// Asks for the game's current state; answered with an EngineSnapshotEvent.
export declare type EngineSnapshotMessage = {
  type: typeof EngineWorkerMessageType.Snapshot;
};

export declare type EngineWorkerMessage = EngineWorkerInitMessage | EngineSnapshotMessage;

export declare type EngineReadyEvent = {
  type: typeof EngineWorkerEventType.Ready;
};

export declare type EngineSnapshotEvent = {
  type: typeof EngineWorkerEventType.Snapshot;
  seed: Uint8Array<ArrayBuffer>; // an encoded protocol.Output
};

export declare type EngineWorkerEvent = EngineReadyEvent | EngineSnapshotEvent;

// #endregion engine worker
