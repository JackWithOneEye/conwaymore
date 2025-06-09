/** @import { CanvasDragState, Globals, PatternDragState } from './types/ui' */
/** @import { CanvasWorkerEvent, CanvasWorkerInitMessage, CanvasWorkerMessage } from './types/worker' */
import { Patterns } from './patterns';
import { getElementByIdOrDie } from './helpers';
import { computed, effect, reactive, signal } from './signals';
import { CanvasWorkerEventType, CanvasWorkerMessageType, Command } from './types/enums';

const globalsEl = getElementByIdOrDie('globals')
if (!globalsEl.textContent) {
  throw new Error('globals element is empty');
}
/** @type {Globals} */
const { WorldSize } = JSON.parse(globalsEl.textContent);

const canvasWorker = new Worker('/assets/js/worker.js', { type: 'module' });

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

    drawGrid: /** @type {HTMLInputElement} */ (getElementByIdOrDie('draw-grid')),
    showAge: /** @type {HTMLInputElement} */ (getElementByIdOrDie('show-age')),

    speed: /** @type {HTMLInputElement} */ (getElementByIdOrDie('speed')),
    speedLabel: getElementByIdOrDie('speed-label'),
  },
  init() {
    App.$.save.removeAttribute('disabled');
    App.$.clear.addEventListener('click', () => canvasWorkerMessage({
      type: CanvasWorkerMessageType.Command,
      cmd: Command.Clear
    }));
    App.$.next.addEventListener('click', () => canvasWorkerMessage({
      type: CanvasWorkerMessageType.Command,
      cmd: Command.Next
    }));
    App.$.playPause.addEventListener('click', () => canvasWorkerMessage({
      type: CanvasWorkerMessageType.Command,
      cmd: App.playback.state() ? Command.Pause : Command.Play
    }));
    App.$.random.addEventListener('click', () => canvasWorkerMessage({
      type: CanvasWorkerMessageType.Command,
      cmd: Command.Randomise
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
      canvasWorkerMessage({ 
        type: CanvasWorkerMessageType.CellSizeChange, 
        cellSize,
        mouseX: -1,
        mouseY: -1
      });
    });

    App.$.cellColour.addEventListener('input', () => {
      App.cellColour.state.update(Number(`0x${App.$.cellColour.value.substring(1)}`));
    });
    App.$.randomColour.addEventListener('click', () => {
      App.cellColour.state.update((Math.random() * 0xffffff) | 0);
    });

    App.$.drawGrid.addEventListener('input', () => {
      App.settings.state.drawGrid = App.$.drawGrid.checked;
    });

    App.$.speed.addEventListener('input', () => {
      canvasWorkerMessage({ type: CanvasWorkerMessageType.SetSpeed, speed: App.speed.convertSpeed(App.$.speed.value) });
    });

    document.addEventListener('mouseup', () => {
      App.moveCanvas.state.mouseState = 'idle';
    });

    const resObs = new ResizeObserver((entries) => {
      const { height, width } = entries[0].contentRect;
      canvasWorkerMessage({ type: CanvasWorkerMessageType.Resize, height, width });
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
      const { coordinates: { bytes, count } } = Patterns[patternType];
      canvasWorkerMessage({
        type: CanvasWorkerMessageType.SetCells,
        count,
        colour: App.cellColour.state(),
        coordinates: bytes,
        originPx: e.offsetX,
        originPy: e.offsetY
      });
    });
    App.$.canvas.addEventListener('wheel', (e) => {
      e.preventDefault();
      const rect = App.$.canvas.getBoundingClientRect();
      const mouseX = e.clientX - rect.left;
      const mouseY = e.clientY - rect.top;
      
      const currentCellSize = Number(App.$.cellSize.value);
      const zoomFactor = currentCellSize <= 5 
        ? (e.deltaY > 0 ? 0.5 : 1.5)
        : (e.deltaY > 0 ? 0.9 : 1.1);
      const newCellSize = Math.max(1, Math.min(100, Math.round(currentCellSize * zoomFactor)));
      
      if (newCellSize !== currentCellSize) {
        App.$.cellSize.value = String(newCellSize);
        App.$.cellSizeLabel.textContent = String(newCellSize);
        canvasWorkerMessage({ 
          type: CanvasWorkerMessageType.CellSizeChange, 
          cellSize: newCellSize,
          mouseX,
          mouseY
        });
      }
    });

    App.playback.state.update(App.$.playPause.textContent === 'PAUSE');
    App.settings.state.drawGrid = App.$.drawGrid.checked;
    App.speed.state.update(App.speed.convertSpeed(App.$.speed.value));

    const cellColourAttr = computed(() => `#${App.cellColour.state().toString(16).padStart(6, '0')}`);

    /**
     * @param {MessageEvent<CanvasWorkerEvent[]>} e 
     */
    canvasWorker.onmessage = (e) => {
      for (const ev of e.data) {
        switch (ev.type) {
          case CanvasWorkerEventType.PlaybackStateChanged:
            App.playback.state.update(ev.playing);
            break;
          case CanvasWorkerEventType.SpeedChanged:
            App.speed.state.update(ev.speed);
            break;
          default:
            console.error('unknown worker event type', ev)
        }
      }
    };

    const osCanvas = App.$.canvas.transferControlToOffscreen();
    canvasWorker.postMessage(/** @type {CanvasWorkerInitMessage} */({
      type: CanvasWorkerMessageType.Init,
      canvas: osCanvas,
      cellSize: Number(App.$.cellSize.value),
      height: App.$.canvasWrapper.offsetHeight,
      width: App.$.canvasWrapper.offsetWidth,
      worldSize: WorldSize
    }), [osCanvas]);

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
      const drawAge = App.settings.state.drawAge
      const drawGrid = App.settings.state.drawGrid;
      canvasWorkerMessage({
        type: CanvasWorkerMessageType.SettingsChange,
        drawAge,
        drawGrid
      });
    });

    effect(() => {
      const speed = App.speed.state();
      App.$.speed.value = `${Math.pow((1000 - speed) * 0.01, 2)}`;
      App.$.speedLabel.textContent = `${speed.toFixed(0)} ms`;
    });
  },
  cellColour: {
    state: signal(0xffffff),
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
      if (st.mouseState === 'dragging' || Math.abs(x) > 7.5 || Math.abs(y) > 7.5) {
        canvasWorkerMessage({ type: CanvasWorkerMessageType.CanvasDrag, dx, dy });
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
          type: CanvasWorkerMessageType.SetCells,
          count: 1,
          colour: App.cellColour.state(),
          coordinates: new Uint8Array([0, 0, 0, 0]),
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
  settings: {
    state: reactive({ drawGrid: true, drawAge: false })
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

canvasWorker.addEventListener('message', App.init, { once: true });
