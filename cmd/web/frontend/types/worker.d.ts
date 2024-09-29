
// #region canvas worker message

export declare type CanvasWorkerInitMessage = {
    type: 'init';
    canvas: OffscreenCanvas;
    cellSize: number;
    height: number;
    width: number;
    worldSize: number;
};

export declare type CanvasDragMessage = {
    type: 'canvasDrag';
    dx: number;
    dy: number;
};

export declare type CellSizeChangeMessage = {
    type: 'cellSizeChange';
    cellSize: number;
};

export const enum Command {
    Next,
    Play,
    Pause,
    Clear,
    Randomise
}
export declare type CommandMessage = {
    type: 'command';
    cmd: Command;
};

export declare type ResizeMessage = {
    type: 'resize';
    height: number;
    width: number;
};

export declare type SetCellsMessage = {
    type: 'setCells';
    colour: number;
    coordinates: number[]; // x | y
    originPx: number;
    originPy: number;
};

export declare type SetSpeedMessage = {
    type: 'setSpeed';
    speed: number;
};

export declare type CanvasWorkerMessage = CanvasWorkerInitMessage
    | CanvasDragMessage
    | CellSizeChangeMessage
    | CommandMessage
    | ResizeMessage
    | SetCellsMessage
    | SetSpeedMessage;

// #endregion canvas worker message

// #region canvas worker event

export declare type PlaybackStateChangedEvent = {
    type: 'playbackStateChanged';
    state: 0 | 1
};

export declare type SpeedChangedEvent = {
    type: 'speedChanged';
    speed: number;
};

export declare type CanvasWorkerEvent = PlaybackStateChangedEvent | SpeedChangedEvent;

// #endregion canvas worker event