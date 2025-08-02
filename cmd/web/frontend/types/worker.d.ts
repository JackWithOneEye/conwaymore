import type { CanvasWorkerEventType, CanvasWorkerMessageType, Command } from './enums';

// #region canvas worker message

export declare type CanvasWorkerInitMessage = {
  type: typeof CanvasWorkerMessageType.Init;
  canvas: OffscreenCanvas;
  cellSize: number;
  height: number;
  width: number;
  worldSize: number;
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

export declare type CanvasWorkerEvent = PlaybackStateChangedEvent | ReadyEvent | SpeedChangedEvent;

// #endregion canvas worker event
