import type { PatternType } from '../patterns';

export declare type Globals = {
    GameID: number,
    GameName: string,
    WorldSize: number,
    WSPath: string
};

export declare type CanvasMouseState = 'idle' | 'down' | 'dragging';
export declare type CanvasDragState = {
    x: number;
    y: number;
    mouseState: CanvasMouseState;
};

export declare type PatternDragState = {
    canvasDragOver: boolean;
    type: PatternType | null;
};
