import type { PatternType } from '../patterns';

export declare type Globals = {
    WorldSize: number
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
