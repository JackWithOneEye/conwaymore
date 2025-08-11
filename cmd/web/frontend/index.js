/** @import { CanvasDragState, Globals, PatternDragState } from './types/ui' */
/** @import { CanvasWorkerEvent, CanvasWorkerInitMessage, CanvasWorkerMessage } from './types/worker' */
import { getElementByIdOrDie, showError, UserPreferences } from './helpers';
import { getPatterns } from './patterns';
import { computed, effect, reactive, signal } from './signals';
import { CanvasWorkerEventType, CanvasWorkerMessageType, Command } from './types/enums';

// Constants for better maintainability
const MAX_ZOOM = 100;
const MIN_ZOOM = 1;
const LOW_ZOOM_THRESHOLD = 5;

const globalsEl = getElementByIdOrDie('globals');
if (!globalsEl.textContent) {
  throw new Error('globals element is empty');
}

let globals;
try {
  globals = JSON.parse(globalsEl.textContent);
} catch (error) {
  showError('Failed to parse global configuration');
  throw error;
}

/** @type {Globals} */
const { WorldSize } = globals;
const Patterns = getPatterns();

const canvasWorker = new Worker('/assets/js/worker.js', { type: 'module' });

/**
 * Send a message to the canvas worker with error handling
 * @param {CanvasWorkerMessage} msg - The message to send to the worker
 */
function canvasWorkerMessage(msg) {
  try {
    canvasWorker.postMessage(msg);
  } catch (error) {
    console.error('Failed to send message to worker:', error);
    showError('Communication error with rendering engine');
  }
}

const App = {
  $: {
    canvas: /** @type {HTMLCanvasElement} */ (document.querySelector('canvas')),
    canvasWrapper: /** @type {HTMLDivElement} */ (getElementByIdOrDie('canvas-wrapper')),

    patternMenu: getElementByIdOrDie('pattern-menu'),
    patternMenuToggle: getElementByIdOrDie('pattern-menu-toggle'),
    patternDragContainer: getElementByIdOrDie('pattern-drag-container'),

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
    // Load user preferences
    const prefs = UserPreferences.load();

    // Apply saved preferences
    if (prefs.cellSize !== undefined) {
      App.$.cellSize.value = String(prefs.cellSize);
      App.$.cellSizeLabel.textContent = String(prefs.cellSize);
    }
    if (prefs.cellColour !== undefined) {
      App.cellColour.state.update(prefs.cellColour);
    }
    if (prefs.showAge !== undefined) {
      App.$.showAge.checked = prefs.showAge;
    }
    if (prefs.drawGrid !== undefined) {
      App.$.drawGrid.checked = prefs.drawGrid;
    }
    if (prefs.speed !== undefined) {
      App.speed.state.update(prefs.speed);
    }

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
    App.$.patternMenuToggle.addEventListener('click', () => {
      patternMenuOpen = !patternMenuOpen;
      if (patternMenuOpen) {
        App.$.patternMenu.dataset.open = 'true';
        App.$.patternMenuToggle.dataset.active = '';
      } else {
        delete App.$.patternMenu.dataset.open;
        delete App.$.patternMenuToggle.dataset.active;
      }
    });
    const patternTemplate = /** @type {HTMLTemplateElement} */ (document.querySelector('template#pattern'));
    const fragment = document.createDocumentFragment();
    for (const [type, pattern] of Patterns.entries()) {
      const patternEl = /** @type {Element} */ (patternTemplate.content.cloneNode(true)).firstElementChild;
      if (!patternEl) {
        throw new Error('failed to clone pattern template')
      }
      const [span] = patternEl.getElementsByTagName('span');
      patternEl.setAttribute('aria-label', `Drag ${pattern.name} pattern to canvas`);
      span.textContent = pattern.name;

        /** @type {HTMLDivElement} */ (patternEl).addEventListener(
        'dragstart',
        (e) => App.dragPattern.start(e, type)
      );
      fragment.appendChild(patternEl);
    }
    App.$.patternMenu.appendChild(fragment);

    App.$.cellSize.addEventListener('input', () => {
      const cellSize = Number(App.$.cellSize.value);
      App.$.cellSizeLabel.textContent = String(cellSize);
      App.$.cellSize.setAttribute('aria-valuetext', `Cell size ${cellSize}`);
      UserPreferences.save({ cellSize });

      canvasWorkerMessage({
        type: CanvasWorkerMessageType.CellSizeChange,
        cellSize,
        mouseX: -1,
        mouseY: -1
      });
    });

    App.$.cellColour.addEventListener('input', () => {
      const colour = Number(`0x${App.$.cellColour.value.substring(1)}`);
      App.cellColour.state.update(colour);
      UserPreferences.save({ cellColour: colour });
    });
    App.$.randomColour.addEventListener('click', () => {
      const colour = (Math.random() * 0xffffff) | 0;
      App.cellColour.state.update(colour);
      UserPreferences.save({ cellColour: colour });
    });

    App.$.drawGrid.addEventListener('input', () => {
      App.settings.state.drawGrid = App.$.drawGrid.checked;
      UserPreferences.save({ drawGrid: App.$.drawGrid.checked });
    });

    App.$.showAge.addEventListener('input', () => {
      App.settings.state.drawAge = App.$.showAge.checked;
      UserPreferences.save({ showAge: App.$.showAge.checked });
    });

    App.$.speed.addEventListener('input', () => {
      const speed = App.speed.convertSpeed(App.$.speed.value);
      App.$.speed.setAttribute('aria-valuetext', `Speed ${speed.toFixed(0)} milliseconds`);
      UserPreferences.save({ speed });
      canvasWorkerMessage({ type: CanvasWorkerMessageType.SetSpeed, speed });
    });

    document.addEventListener('mouseup', () => {
      App.moveCanvas.state.mouseState = 'idle';
    });

    const resObs = new ResizeObserver((entries) => {
      const { height, width } = entries[0].contentRect;
      canvasWorkerMessage({
        type: CanvasWorkerMessageType.Resize,
        height,
        width
      });
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

      if (e.touches.length === 1) {
        // Single touch - drag
        App.moveCanvas.start(e.targetTouches[0].clientX, e.targetTouches[0].clientY);
      } else if (e.touches.length === 2) {
        // Two finger touch - prepare for pinch zoom
        App.setupPinchZoom(e);
      }
    });
    App.$.canvas.addEventListener('mousemove', (e) => {
      App.moveCanvas.drag(e.x, e.y);
    });
    App.$.canvas.addEventListener('touchmove', (e) => {
      e.preventDefault();
      e.stopPropagation();

      if (e.touches.length === 1) {
        App.moveCanvas.drag(e.targetTouches[0].clientX, e.targetTouches[0].clientY);
      } else if (e.touches.length === 2) {
        App.handlePinchZoom(e);
      }
    });
    App.$.canvas.addEventListener('mouseup', (e) => {
      e.preventDefault();
      e.stopPropagation();
      App.moveCanvas.end(e.offsetX, e.offsetY);
    });
    App.$.canvas.addEventListener('touchend', (e) => {
      e.preventDefault();
      e.stopPropagation();

      if (e.changedTouches.length === 1 && e.touches.length === 0) {
        const bcr = /** @type {HTMLElement} */ (e.target).getBoundingClientRect();
        App.moveCanvas.end(
          e.changedTouches[0].pageX - bcr.left,
          e.changedTouches[0].pageY - bcr.top
        );
      }
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
      canvasWorkerMessage({
        type: CanvasWorkerMessageType.SetPattern,
        patternType,
        colour: App.cellColour.state(),
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
      const zoomFactor = currentCellSize <= LOW_ZOOM_THRESHOLD
        ? (e.deltaY > 0 ? 0.5 : 1.5)
        : (e.deltaY > 0 ? 0.9 : 1.1);
      const newCellSize = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, Math.round(currentCellSize * zoomFactor)));

      if (newCellSize !== currentCellSize) {
        App.$.cellSize.value = String(newCellSize);
        App.$.cellSizeLabel.textContent = String(newCellSize);
        UserPreferences.save({ cellSize: newCellSize });

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
    App.settings.state.drawAge = App.$.showAge.checked;
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
            console.error('unknown worker event type', ev);
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
        App.$.playPause.setAttribute('aria-label', 'Pause simulation');
      } else {
        App.$.next.disabled = false;
        App.$.playPause.textContent = 'PLAY';
        App.$.playPause.setAttribute('aria-label', 'Start simulation');
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

  // Pinch zoom support for mobile
  pinchState: {
    initialDistance: 0,
    initialCellSize: 0,
    centerX: 0,
    centerY: 0
  },

  /**
   * Initialize pinch zoom gesture tracking
   * @param {TouchEvent} e - The touch start event with two touches
   */
  setupPinchZoom(e) {
    const touch1 = e.touches[0];
    const touch2 = e.touches[1];

    const dx = touch2.clientX - touch1.clientX;
    const dy = touch2.clientY - touch1.clientY;

    App.pinchState.initialDistance = Math.sqrt(dx * dx + dy * dy);
    App.pinchState.initialCellSize = Number(App.$.cellSize.value);
    App.pinchState.centerX = (touch1.clientX + touch2.clientX) / 2;
    App.pinchState.centerY = (touch1.clientY + touch2.clientY) / 2;
  },

  /**
   * Handle pinch zoom gesture for mobile devices
   * @param {TouchEvent} e - The touch move event with two touches
   */
  handlePinchZoom(e) {
    if (App.pinchState.initialDistance === 0) {
      return;
    }

    const touch1 = e.touches[0];
    const touch2 = e.touches[1];

    const dx = touch2.clientX - touch1.clientX;
    const dy = touch2.clientY - touch1.clientY;
    const currentDistance = Math.sqrt(dx * dx + dy * dy);

    const scale = currentDistance / App.pinchState.initialDistance;
    const newCellSize = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, Math.round(App.pinchState.initialCellSize * scale)));

    if (newCellSize !== Number(App.$.cellSize.value)) {
      App.$.cellSize.value = String(newCellSize);
      App.$.cellSizeLabel.textContent = String(newCellSize);
      UserPreferences.save({ cellSize: newCellSize });

      const rect = App.$.canvas.getBoundingClientRect();
      canvasWorkerMessage({
        type: CanvasWorkerMessageType.CellSizeChange,
        cellSize: newCellSize,
        mouseX: App.pinchState.centerX - rect.left,
        mouseY: App.pinchState.centerY - rect.top
      });
    }
  },
  cellColour: {
    state: signal(0xffffff),
  },
  dragPattern: {
    state: reactive(/** @type {PatternDragState} */({ canvasDragOver: false, type: null })),
    /**
     * @param {DragEvent} event 
     * @param {string} type 
     */
    start(event, type) {
      if (event.dataTransfer) {
        event.dataTransfer.setData('application/conwaymore', type)
        event.dataTransfer.effectAllowed = 'move'

        const pattern = /** @type {import('./patterns').Pattern} */ (Patterns.get(type));
        const cellSize = Math.round(Number(App.$.cellSize.value));

        let maxX = -Infinity, maxY = -Infinity;
        for (const cell of pattern.cells) {
          maxX = Math.max(maxX, cell.x);
          maxY = Math.max(maxY, cell.y);
        }
        const width = (maxX + 1) * cellSize;
        const height = (maxY + 1) * cellSize;
        App.$.patternDragContainer.style.cssText = `width: ${width}px; height: ${height}px;`;

        const fragment = document.createDocumentFragment();
        for (const cell of pattern.cells) {
          const cellDiv = document.createElement('div');
          const x = cell.x * cellSize;
          const y = cell.y * cellSize;

          cellDiv.style.cssText = `
            position: absolute;
            left: ${x}px;
            top: ${y}px;
            width: ${cellSize}px;
            height: ${cellSize}px;
            background-color: ${App.$.cellColour.value};
          `;
          fragment.appendChild(cellDiv);
        }
        App.$.patternDragContainer.appendChild(fragment);

        event.dataTransfer.setDragImage(
          App.$.patternDragContainer,
          pattern.center_x * cellSize,
          pattern.center_y * cellSize
        );
        requestAnimationFrame(() => {
          App.$.patternDragContainer.textContent = '';
        })
      }
      App.dragPattern.state.type = type;
      document.body.style.userSelect = 'none';
      document.addEventListener('drop', App.dragPattern.end)
    },
    /**
     * Clean up pattern drag state and event listeners
     */
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
     * Convert speed slider value to milliseconds delay
     * @param {string} attrValue - The slider value as a string
     * @returns {number} - The delay in milliseconds (1-1000)
     */
    convertSpeed(attrValue) {
      return Math.max(1, 1000 - Math.sqrt(Number(attrValue)) * 100);
    }
  }
};

canvasWorker.addEventListener('message', App.init, { once: true });
