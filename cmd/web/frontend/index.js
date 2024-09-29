/** @import { CanvasDragState, Globals, PatternDragState } from './types/ui' */
/** @import { CanvasWorkerEvent, CanvasWorkerInitMessage, CanvasWorkerMessage, Command } from './types/worker' */
import { Patterns } from './patterns';
import { getElementByIdOrDie } from './helpers';
import { computed, effect, reactive, signal } from './signals';

const globalsEl = getElementByIdOrDie('globals')
if (!globalsEl.textContent) {
    throw new Error('globals element is empty');
}
/** @type {Globals} */
const { WorldSize } = JSON.parse(globalsEl.textContent);

const canvasWorker = new Worker('/assets/js/worker/worker.js', { type: 'module' });

/**
 * @param {CanvasWorkerMessage} msg 
 */
function canvasWorkerMessage(msg) {
    canvasWorker.postMessage(msg);
}

const App = {
    $: {
        canvas: /** @type {HTMLCanvasElement} */ (document.querySelector('canvas')),
        canvasWrapper: /** @type {HTMLDivElement} */ (getElementByIdOrDie('canvas-wrapper')),

        patternMenu: getElementByIdOrDie('pattern-menu'),
        pattenMenuToggle: getElementByIdOrDie('pattern-menu-toggle'),

        clear: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('clear')),
        next: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('next')),
        playPause: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('play-pause')),
        save: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('save-game')),
        random: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('random')),

        cellColour: /** @type {HTMLInputElement} */ (getElementByIdOrDie('cell-colour')),
        randomColour: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('random-colour')),

        cellSize: /** @type {HTMLInputElement} */ (getElementByIdOrDie('cell-size')),
        cellSizeLabel: getElementByIdOrDie('cell-size-label'),

        speed: /** @type {HTMLInputElement} */ (getElementByIdOrDie('speed')),
        speedLabel: getElementByIdOrDie('speed-label'),
    },
    init() {
        App.$.save.removeAttribute('disabled');
        App.$.clear.addEventListener('click', () => canvasWorkerMessage({
            type: 'command',
            /** @type {Command.Clear} */
            cmd: 3
        }));
        App.$.next.addEventListener('click', () => canvasWorkerMessage({
            type: 'command',
            /** @type {Command.Next} */
            cmd: 0
        }));
        App.$.playPause.addEventListener('click', () => canvasWorkerMessage({
            type: 'command',
            /** @type {Command.Play | Command.Pause} */
            cmd: App.playback.state() ? 2 : 1
        }));
        App.$.random.addEventListener('click', () => canvasWorkerMessage({
            type: 'command',
            /** @type {Command.Randomise} */
            cmd: 4
        }));

        let patternMenuOpen = false;
        App.$.pattenMenuToggle.addEventListener('click', () => {
            patternMenuOpen = !patternMenuOpen;
            if (patternMenuOpen) {
                App.$.patternMenu.dataset.open = 'true';
                App.$.pattenMenuToggle.dataset.active = '';
            } else {
                delete App.$.patternMenu.dataset.open;
                delete App.$.pattenMenuToggle.dataset.active;
            }
        });
        const patternTemplate = /** @type {HTMLTemplateElement} */ (getElementByIdOrDie('pattern-template'));
        const fragment = document.createDocumentFragment();
        for (const [type, pattern] of Object.entries(Patterns)) {
            const patternEl = /** @type {Element} */ (patternTemplate.content.cloneNode(true)).firstElementChild;
            if (!patternEl) {
                throw new Error('failed to clone pattern template')
            }
            const [span] = patternEl.getElementsByTagName('span');
            span.textContent = pattern.name;
            /** @type {HTMLDivElement} */ (patternEl).addEventListener(
                'dragstart',
                (e) => App.dragPattern.start(e, /** @type {import('./patterns').PatternType} */(type))
            );
            fragment.appendChild(patternEl);
        }
        App.$.patternMenu.appendChild(fragment);

        App.$.cellSize.addEventListener('input', () => {
            const cellSize = Number(App.$.cellSize.value);
            App.$.cellSizeLabel.textContent = App.$.cellSize.value;
            canvasWorkerMessage({ type: 'cellSizeChange', cellSize });
        });

        App.$.cellColour.addEventListener('input', () => {
            App.cellColour.state.update(Number(`0x${App.$.cellColour.value.substring(1)}`));
        });
        App.$.randomColour.addEventListener('click', () => {
            App.cellColour.state.update((Math.random() * 0xffffff) | 0);
        });

        App.$.speed.addEventListener('input', () => {
            canvasWorkerMessage({ type: 'setSpeed', speed: App.speed.convertSpeed(App.$.speed.value) });
        });

        document.addEventListener('mouseup', () => {
            App.moveCanvas.state.mouseState = 'idle';
        });

        const resObs = new ResizeObserver((entries) => {
            const { height, width } = entries[0].contentRect;
            canvasWorkerMessage({ type: 'resize', height, width });
        });
        resObs.observe(App.$.canvasWrapper);

        App.$.canvas.addEventListener('mousedown', (e) => {
            e.preventDefault();
            e.stopPropagation();
            App.moveCanvas.start(e.x, e.y);
        });
        App.$.canvas.addEventListener('touchstart', (e) => {
            e.preventDefault();
            e.stopPropagation();
            App.moveCanvas.start(e.targetTouches[0].clientX, e.targetTouches[0].clientY);
        });
        App.$.canvas.addEventListener('mousemove', (e) => {
            App.moveCanvas.drag(e.x, e.y);
        });
        App.$.canvas.addEventListener('touchmove', (e) => {
            App.moveCanvas.drag(e.targetTouches[0].clientX, e.targetTouches[0].clientY);
        });
        App.$.canvas.addEventListener('mouseup', (e) => {
            e.preventDefault();
            e.stopPropagation();
            App.moveCanvas.end(e.offsetX, e.offsetY);
        });
        App.$.canvas.addEventListener('touchend', (e) => {
            e.preventDefault();
            e.stopPropagation();
            const bcr = /** @type {HTMLElement} */ (e.target).getBoundingClientRect();
            App.moveCanvas.end(e.changedTouches[0].pageX - bcr.left, e.changedTouches[0].pageY - bcr.top);
        });
        App.$.canvas.addEventListener('dragover', (e) => {
            e.preventDefault();
            const st = App.dragPattern.state;
            if (!st.type) {
                return;
            }
            st.canvasDragOver = true;
            if (e.dataTransfer) {
                e.dataTransfer.dropEffect = 'move';
            }
        });
        App.$.canvas.addEventListener('dragleave', () => {
            App.dragPattern.state.canvasDragOver = false;
        });
        App.$.canvas.addEventListener('drop', (e) => {
            const patternType = App.dragPattern.state.type;
            if (!patternType) {
                return;
            }
            const { coordinates } = Patterns[patternType];
            canvasWorkerMessage({
                type: 'setCells',
                colour: App.cellColour.state(),
                coordinates,
                originPx: e.offsetX,
                originPy: e.offsetY
            });
        });

        App.playback.state.update(App.$.playPause.textContent === 'PAUSE');
        App.speed.state.update(App.speed.convertSpeed(App.$.speed.value));

        const cellColourAttr = computed(() => `#${App.cellColour.state().toString(16).padStart(6, '0')}`);
        effect(() => {
            const attr = cellColourAttr();
            App.$.cellColour.value = attr;
            App.$.randomColour.textContent = attr;
        });

        effect(() => {
            if (App.dragPattern.state.canvasDragOver) {
                App.$.canvas.dataset.dragover = '';
            } else {
                delete App.$.canvas.dataset.dragover;
            }
        });

        effect(() => {
            if (App.moveCanvas.state.mouseState === 'dragging') {
                App.$.canvas.dataset.dragging = '';
            } else {
                delete App.$.canvas.dataset.dragging;
            }
        });

        effect(() => {
            if (App.playback.state()) {
                App.$.next.disabled = true;
                App.$.playPause.textContent = 'PAUSE';
            } else {
                App.$.next.disabled = false;
                App.$.playPause.textContent = 'PLAY';
            }
        });

        effect(() => {
            const speed = App.speed.state();
            App.$.speed.value = `${Math.pow((1000 - speed) * 0.01, 2)}`;
            App.$.speedLabel.textContent = `${speed.toFixed(0)} ms`;
        });

        /**
         * @param {MessageEvent<CanvasWorkerEvent[]>} arg 
         */
        canvasWorker.onmessage = ({ data }) => {
            for (const ev of data) {
                switch (ev.type) {
                    case 'playbackStateChanged':
                        App.playback.state.update(ev.state === 1);
                        break;
                    case 'speedChanged':
                        App.speed.state.update(ev.speed);
                        break;
                    default:
                        console.error('unknown worker event type', ev)
                }
            }
        };

        const osCanvas = App.$.canvas.transferControlToOffscreen();
        canvasWorker.postMessage(/** @type {CanvasWorkerInitMessage} */({
            type: 'init',
            canvas: osCanvas,
            cellSize: Number(App.$.cellSize.value),
            height: App.$.canvasWrapper.offsetHeight,
            width: App.$.canvasWrapper.offsetWidth,
            worldSize: WorldSize
        }), [osCanvas]);
    },
    cellColour: {
        state: signal(0),
    },
    dragPattern: {
        state: reactive(/** @type {PatternDragState} */({ canvasDragOver: false, type: null })),
        /**
         * @param {DragEvent} event 
         * @param {import('./patterns').PatternType} type 
         */
        start(event, type) {
            if (event.dataTransfer) {
                event.dataTransfer.setData('application/conwaymore', type)
                event.dataTransfer.effectAllowed = 'move'
            }
            App.dragPattern.state.type = type;
            document.body.style.userSelect = 'none';
            document.addEventListener('drop', App.dragPattern.end)
        },
        end() {
            App.dragPattern.state.canvasDragOver = false;
            App.dragPattern.state.type = null;
            document.body.style.userSelect = '';
            document.removeEventListener('drop', App.dragPattern.end)
        }
    },
    moveCanvas: {
        state: reactive(/** @type {CanvasDragState} */({ x: 0, y: 0, mouseState: 'idle' })),
        /**
         * @param {number} x 
         * @param {number} y 
         */
        start(x, y) {
            const st = App.moveCanvas.state;
            st.x = x;
            st.y = y;
            st.mouseState = 'down';
        },
        /**
         * @param {number} x 
         * @param {number} y 
         */
        drag(x, y) {
            const st = App.moveCanvas.state;
            if (st.mouseState === 'idle') {
                return;
            }
            const dx = x - st.x;
            const dy = y - st.y;
            if (st.mouseState === 'dragging' || Math.abs(x) > 5 || Math.abs(y) > 5) {
                canvasWorkerMessage({ type: 'canvasDrag', dx, dy });
                st.x = x;
                st.y = y;
                st.mouseState = 'dragging';
            }
        },
        /**
         * @param {number} x 
         * @param {number} y 
         */
        end(x, y) {
            const st = App.moveCanvas.state;
            if (st.mouseState === 'down') {
                canvasWorkerMessage({
                    type: 'setCells',
                    colour: App.cellColour.state(),
                    coordinates: [0],
                    originPx: x,
                    originPy: y
                });
            }
            st.mouseState = 'idle';
        }
    },
    playback: {
        state: signal(false),
    },
    speed: {
        state: signal(0),
        /**
         * @param {string} attrValue 
         */
        convertSpeed(attrValue) {
            return Math.max(1, 1000 - Math.sqrt(Number(attrValue)) * 100);
        }
    }
};

App.init();

