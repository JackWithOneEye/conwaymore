/** @import { CanvasWorkerMessage, CanvasWorkerInitMessage, CanvasWorkerEvent } from '../types/worker' */
import CanvasDrawer from './canvas-drawer';


/** @type {WebSocket} */
let ws;

/** @type {CanvasDrawer} */
let drawer;

/** @type {{ data: Uint32Array; length: number; }} */
let cells;

/** @type {number | null} */
let drawHandle = null;

function draw() {
    if (drawHandle != null) {
        cancelAnimationFrame(drawHandle);
    }
    drawHandle = requestAnimationFrame(() => {
        drawer.draw(cells);
        drawHandle = null;
    });
}

let initialised = false;
/**
 * 
 * @param {CanvasWorkerInitMessage} msg 
 */
function init({ canvas, cellSize, height, width, worldSize }) {
    if (initialised) {
        throw new Error(`[worker.js] Already initialised!`);
    }

    cells = { data: new Uint32Array(worldSize * worldSize * 2), length: 0 };
    drawer = new CanvasDrawer(worldSize, canvas, scaleCellSize(cellSize), height, width);

    ws = new WebSocket('/play');
    ws.addEventListener('open', (event) => {
        console.log('WS CONN OPEN', event)
    });
    ws.addEventListener('message', async (event) => {
        const blob = /** @type {Blob} */(event.data);
        const buf = await blob.arrayBuffer();
        const data = new Uint8Array(buf);
        // console.log("Message from server ", data);

        postMessage(/** @type {CanvasWorkerEvent[]} */([
            {
                type: 'playbackStateChanged',
                state: data[0]
            },
            {
                type: 'speedChanged',
                speed: ((data[1] << 8) & 0xff00) | (data[2] & 0x00ff)
            }
        ]));

        const cellsCount = ((data[3] << 16) & 0xff0000) | ((data[4] << 8) & 0x00ff00) | (data[5] & 0x0000ff);

        let di = 0;
        let bi = 6;
        for (let i = 0; i < cellsCount; i++) {
            cells.data[di++] = (
                ((data[bi++] << 24) & 0xff000000) | ((data[bi++] << 16) & 0x00ff0000) // x
                | ((data[bi++] << 8) & 0x0000ff00) | (data[bi++] & 0x000000ff) // y
            );

            cells.data[di++] = ((data[bi++] << 16) & 0xff0000) | ((data[bi++] << 8) & 0x00ff00) | (data[bi++] & 0x0000ff);
        }

        cells.length = cellsCount;
        draw();
    });

    initialised = true;
}

/** @param {number} cellSize */
function scaleCellSize(cellSize) {
    return Math.round(2 + 0.38 * cellSize);
}

/**
 * @param {MessageEvent<CanvasWorkerMessage>} arg
 */
self.onmessage = ({ data }) => {
    switch (data.type) {
        case 'init':
            init(data)
            break;
        case 'canvasDrag':
            drawer.incrementOffset(data.dx, data.dy);
            break;
        case 'cellSizeChange':
            drawer.cellSize = scaleCellSize(data.cellSize);
            break;
        case 'command':
            ws.send(new Uint8Array([1, data.cmd]));
            return;
        case 'resize':
            drawer.setDimensions(data.height, data.width);
            break;
        case 'setCells':
            const { colour, coordinates, originPx, originPy } = data;
            const [x, y] = drawer.pixelToCellCoord(originPx, originPy);
            const msg = new Uint8Array(5 + coordinates.length * 7);
            let msgi = 0
            msg[msgi++] = 4;
            msg[msgi++] = 0;
            msg[msgi++] = 0;
            msg[msgi++] = 0;
            msg[msgi++] = coordinates.length;
            for (const coord of coordinates) {
                const cx = x + ((coord >> 16) & 0xffff);
                const cy = y + (coord & 0xffff);
                msg[msgi++] = (cx >> 8) & 0x00ff;
                msg[msgi++] = cx & 0x00ff;
                msg[msgi++] = (cy >> 8) & 0x00ff;
                msg[msgi++] = cy & 0x00ff;
                msg[msgi++] = (colour >> 16) & 0x0000ff;
                msg[msgi++] = (colour >> 8) & 0x0000ff;
                msg[msgi++] = colour & 0x0000ff;
            }
            ws.send(msg);
            return;
        case 'setSpeed':
            ws.send(new Uint8Array([2, 0, (data.speed >> 8) & 0x00ff, data.speed & 0x00ff]));
            return;
        default:
            console.error('[worker.js] unknown message type', data);
            return;
    }

    draw();
};

