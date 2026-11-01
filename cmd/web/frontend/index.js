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
const SPEED_MIN = 1;
const SPEED_MAX = 100;
const TOUCH_DRAG_HOLD_MS = 300;
const TOUCH_DRAG_SLOP_PX = 8;
const TOUCH_DRAG_LIFT_PX = 56; // the touch drag preview floats above the finger so it stays visible
const DIAL_SWEEP_DEG = 270;
const DIAL_CENTRE = 11; // pixel at the middle of the dial's 23x23 grid
const DIAL_RADIUS = 10;
const DIAL_POINTER_LENGTH = 7;

/**
 * Pixels of the dial's arc: a midpoint-circle raster around the centre pixel, limited to the sweep.
 * Each pixel keeps its angle (0° = up, clockwise positive) so the lit part can be split off.
 * @type {{x: number, y: number, angle: number}[]}
 */
const DIAL_ARC = (() => {
  /** @type {Map<string, [number, number]>} */
  const offsets = new Map();
  let x = DIAL_RADIUS, y = 0, err = 1 - DIAL_RADIUS;
  while (x >= y) {
    for (const [dx, dy] of [[x, y], [y, x], [-y, x], [-x, y], [-x, -y], [-y, -x], [y, -x], [x, -y]]) {
      offsets.set(`${dx},${dy}`, [dx, dy]);
    }
    y++;
    if (err < 0) {
      err += 2 * y + 1;
    } else {
      x--;
      err += 2 * (y - x) + 1;
    }
  }
  return [...offsets.values()]
    .map(([dx, dy]) => ({ x: DIAL_CENTRE + dx, y: DIAL_CENTRE + dy, angle: Math.atan2(dx, -dy) * 180 / Math.PI }))
    .filter(({ angle }) => Math.abs(angle) <= DIAL_SWEEP_DEG / 2);
})();

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
    patternMenuIcon: getElementByIdOrDie('pattern-menu-icon'),
    patternList: getElementByIdOrDie('pattern-list'),
    patternDragContainer: getElementByIdOrDie('pattern-drag-container'),
    patternGhost: getElementByIdOrDie('pattern-ghost'),
    patternPanel: getElementByIdOrDie('pattern-panel'),

    clear: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('clear')),
    next: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('next')),
    playPause: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('play-pause')),
    playPauseLabel: getElementByIdOrDie('play-pause-label'),
    save: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('save-game')),
    saveLabel: getElementByIdOrDie('save-label'),
    themeToggle: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('theme-toggle')),
    random: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('random')),

    cellColour: /** @type {HTMLInputElement} */ (getElementByIdOrDie('cell-colour')),
    randomColour: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('random-colour')),
    colourToggle: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('cell-colour-toggle')),
    colourPreview: getElementByIdOrDie('cell-colour-preview'),
    colourPopover: getElementByIdOrDie('colour-popover'),
    colourPresets: getElementByIdOrDie('colour-presets'),
    colourHex: /** @type {HTMLInputElement} */ (getElementByIdOrDie('colour-hex')),
    colourMore: /** @type {HTMLButtonElement} */ (getElementByIdOrDie('colour-more')),

    zoom: /** @type {HTMLInputElement} */ (getElementByIdOrDie('zoom')),
    zoomLabel: getElementByIdOrDie('zoom-label'),

    drawGrid: /** @type {HTMLInputElement} */ (getElementByIdOrDie('draw-grid')),
    showAge: /** @type {HTMLInputElement} */ (getElementByIdOrDie('show-age')),

    speed: /** @type {HTMLInputElement} */ (getElementByIdOrDie('speed')),
    speedLabel: getElementByIdOrDie('speed-label'),
    speedDial: getElementByIdOrDie('speed-dial'),
    speedArc: getElementByIdOrDie('speed-arc'),
    speedArcLit: getElementByIdOrDie('speed-arc-lit'),
    speedPointer: getElementByIdOrDie('speed-pointer'),
  },
  init() {
    // Load user preferences
    const prefs = UserPreferences.load();

    // Apply saved preferences
    App.zoom.set(prefs.cellSize ?? Number(App.$.zoom.value));
    App.cellColour.state.update(prefs.cellColour ?? Number(`0x${App.$.cellColour.value.substring(1)}`));
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
    App.$.save.addEventListener('htmx:afterRequest', (e) => {
      if (!(/** @type {CustomEvent} */ (e)).detail.successful) {
        showError('Saving failed');
        return;
      }
      App.$.saveLabel.textContent = 'Saved';
      setTimeout(() => {
        App.$.saveLabel.textContent = 'Save';
      }, 1500);
    });

    App.$.themeToggle.addEventListener('click', () => {
      const theme = App.theme.state() === 'dark' ? 'light' : 'dark';
      App.theme.apply(theme);
      UserPreferences.save({ theme });
    });
    matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
      if (!UserPreferences.load().theme) {
        App.theme.apply(e.matches ? 'dark' : 'light');
      }
    });
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

    App.$.patternMenuToggle.addEventListener('click', () => {
      const open = App.$.patternMenu.hidden;
      App.$.patternMenu.hidden = !open;
      App.$.patternMenuToggle.setAttribute('aria-expanded', String(open));
      App.$.patternMenuIcon.textContent = open ? '-' : '+';
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

      // Pixel preview: one 1x1 square per cell, with a one-cell margin.
      const [preview] = patternEl.getElementsByTagName('svg');
      let maxX = 0, maxY = 0, d = '';
      for (const { x, y } of pattern.cells) {
        maxX = Math.max(maxX, x);
        maxY = Math.max(maxY, y);
        d += `M${x} ${y}h1v1h-1z`;
      }
      preview.setAttribute('viewBox', `-1 -1 ${maxX + 3} ${maxY + 3}`);
      preview.firstElementChild?.setAttribute('d', d);

      // Mouse drags use native drag and drop; touch and pen use a pointer-driven drag,
      // because mobile browsers do not start native drags from touch.
      /** @type {HTMLDivElement} */ (patternEl).addEventListener(
        'dragstart',
        (e) => App.dragPattern.start(e, type)
      );
      /** @type {HTMLDivElement} */ (patternEl).addEventListener('pointerdown', (e) => {
        if (e.pointerType !== 'mouse') {
          App.dragPattern.touchStart(e, type, /** @type {HTMLElement} */ (patternEl));
        }
      });
      // Once a touch drag has started, keep the browser from scrolling the list instead.
      patternEl.addEventListener('touchmove', (e) => {
        if (App.dragPattern.touch?.active) {
          e.preventDefault();
        }
      }, { passive: false });
      fragment.appendChild(patternEl);
    }
    App.$.patternList.appendChild(fragment);

    App.$.zoom.addEventListener('input', () => {
      const cellSize = Number(App.$.zoom.value);
      App.zoom.set(cellSize);
      UserPreferences.save({ cellSize });

      canvasWorkerMessage({
        type: CanvasWorkerMessageType.CellSizeChange,
        cellSize,
        mouseX: -1,
        mouseY: -1
      });
    });

    App.$.cellColour.addEventListener('input', () => {
      App.cellColour.set(Number(`0x${App.$.cellColour.value.substring(1)}`));
    });
    App.$.randomColour.addEventListener('click', () => {
      App.cellColour.set((Math.random() * 0xffffff) | 0);
    });
    App.$.colourPresets.addEventListener('click', (e) => {
      const preset = /** @type {HTMLElement} */ (e.target).closest('[data-colour]');
      if (preset instanceof HTMLElement && preset.dataset.colour) {
        App.cellColour.set(Number(`0x${preset.dataset.colour.substring(1)}`));
        App.$.colourPopover.hidePopover();
      }
    });
    App.$.colourHex.addEventListener('input', () => {
      const match = /^#?([0-9a-f]{6})$/i.exec(App.$.colourHex.value.trim());
      App.$.colourHex.setAttribute('aria-invalid', String(!match));
      if (match) {
        App.cellColour.set(Number(`0x${match[1]}`));
      }
    });
    App.$.colourHex.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && App.$.colourHex.getAttribute('aria-invalid') !== 'true') {
        // Without this, the Enter would also press the swatch button that regains focus and reopen the popover.
        e.preventDefault();
        App.$.colourPopover.hidePopover();
      }
    });
    App.$.colourHex.addEventListener('blur', () => {
      // Drop an unfinished value and show the colour in use.
      App.$.colourHex.value = cellColourAttr();
      App.$.colourHex.removeAttribute('aria-invalid');
    });
    App.$.colourMore.addEventListener('click', () => {
      App.$.cellColour.showPicker();
    });
    App.$.colourPopover.addEventListener('beforetoggle', (e) => {
      const open = /** @type {ToggleEvent} */ (e).newState === 'open';
      App.$.colourToggle.setAttribute('aria-expanded', String(open));
      if (!open) {
        return;
      }
      // Open above the control deck, centred on the swatch.
      const rect = App.$.colourToggle.getBoundingClientRect();
      const deckTop = App.$.colourToggle.closest('section')?.getBoundingClientRect().top ?? rect.top;
      Object.assign(App.$.colourPopover.style, {
        inset: 'auto',
        bottom: `${innerHeight - deckTop + 14}px`,
        left: `${rect.left + rect.width / 2}px`,
        transform: 'translateX(-50%)'
      });
    });
    // `toggle` fires asynchronously, after the popover is shown.
    App.$.colourPopover.addEventListener('toggle', (e) => {
      if (/** @type {ToggleEvent} */ (e).newState !== 'open') {
        return;
      }
      // Keep it inside the viewport on narrow screens.
      const { left, width } = App.$.colourPopover.getBoundingClientRect();
      const margin = 12;
      const clamped = Math.max(margin, Math.min(left, innerWidth - width - margin));
      if (clamped !== left) {
        Object.assign(App.$.colourPopover.style, { left: `${clamped}px`, transform: 'none' });
      }
      if (!App.$.colourPopover.contains(document.activeElement)) {
        const selected = App.$.colourPresets.querySelector('[aria-pressed="true"]');
        (selected instanceof HTMLElement ? selected : App.$.colourHex).focus();
      }
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
      App.speed.renderDial();
      UserPreferences.save({ speed });
      canvasWorkerMessage({ type: CanvasWorkerMessageType.SetSpeed, speed });
    });

    // The dial drives the visually hidden range input, which keeps keyboard and screen reader support.
    App.$.speedDial.addEventListener('pointerdown', (e) => {
      e.preventDefault();
      App.$.speedDial.setPointerCapture(e.pointerId);
      App.$.speed.focus({ preventScroll: true });
      App.speed.setFromPointer(e);
    });
    App.$.speedDial.addEventListener('pointermove', (e) => {
      if (App.$.speedDial.hasPointerCapture(e.pointerId)) {
        App.speed.setFromPointer(e);
      }
    });
    App.$.speedDial.addEventListener('wheel', (e) => {
      e.preventDefault();
      App.speed.setValue(Number(App.$.speed.value) + (e.deltaY < 0 ? 2 : -2));
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
      if (patternType) {
        App.dragPattern.drop(patternType, e.offsetX, e.offsetY);
      }
    });
    App.$.canvas.addEventListener('wheel', (e) => {
      e.preventDefault();
      const rect = App.$.canvas.getBoundingClientRect();
      const mouseX = e.clientX - rect.left;
      const mouseY = e.clientY - rect.top;

      const currentCellSize = Number(App.$.zoom.value);
      const zoomFactor = currentCellSize <= LOW_ZOOM_THRESHOLD
        ? (e.deltaY > 0 ? 0.5 : 1.5)
        : (e.deltaY > 0 ? 0.9 : 1.1);
      const newCellSize = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, Math.round(currentCellSize * zoomFactor)));

      if (newCellSize !== currentCellSize) {
        App.zoom.set(newCellSize);
        UserPreferences.save({ cellSize: newCellSize });

        canvasWorkerMessage({
          type: CanvasWorkerMessageType.CellSizeChange,
          cellSize: newCellSize,
          mouseX,
          mouseY
        });
      }
    });

    App.playback.state.update('playing' in App.$.playPause.dataset);
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
      cellSize: Number(App.$.zoom.value),
      height: App.$.canvasWrapper.offsetHeight,
      width: App.$.canvasWrapper.offsetWidth,
      worldSize: WorldSize
    }), [osCanvas]);

    effect(() => {
      const attr = cellColourAttr();
      App.$.cellColour.value = attr;
      App.$.colourPreview.style.background = attr;
      App.$.colourToggle.title = attr;
      if (document.activeElement !== App.$.colourHex) {
        App.$.colourHex.value = attr;
      }
      for (const preset of App.$.colourPresets.querySelectorAll('[data-colour]')) {
        preset.setAttribute('aria-pressed', String(/** @type {HTMLElement} */ (preset).dataset.colour === attr));
      }
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
        App.$.playPause.dataset.playing = '';
        App.$.playPauseLabel.textContent = 'PAUSE';
        App.$.playPause.setAttribute('aria-label', 'Pause simulation');
      } else {
        App.$.next.disabled = false;
        delete App.$.playPause.dataset.playing;
        App.$.playPauseLabel.textContent = 'PLAY';
        App.$.playPause.setAttribute('aria-label', 'Start simulation');
      }
    });

    effect(() => {
      App.$.themeToggle.setAttribute('aria-pressed', String(App.theme.state() === 'dark'));
    });

    effect(() => {
      const drawAge = App.settings.state.drawAge
      const drawGrid = App.settings.state.drawGrid;
      App.theme.state(); // re-read the canvas colours whenever the theme changes
      const styles = getComputedStyle(document.documentElement);
      /** @param {string} name */
      const cssColour = (name) => parseInt(styles.getPropertyValue(name).trim().slice(1), 16);
      canvasWorkerMessage({
        type: CanvasWorkerMessageType.SettingsChange,
        drawAge,
        drawGrid,
        backgroundColour: cssColour('--canvas-bg'),
        gridColour: cssColour('--canvas-grid')
      });
    });

    effect(() => {
      const speed = App.speed.state();
      App.$.speed.value = `${Math.pow((1000 - speed) * 0.01, 2)}`;
      App.$.speedLabel.textContent = `${speed.toFixed(0)} ms`;
      App.speed.renderDial();
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
    App.pinchState.initialCellSize = Number(App.$.zoom.value);
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

    if (newCellSize !== Number(App.$.zoom.value)) {
      App.zoom.set(newCellSize);
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
    /**
     * Use a colour for new cells and remember it
     * @param {number} colour
     */
    set(colour) {
      App.cellColour.state.update(colour);
      UserPreferences.save({ cellColour: colour });
    },
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
        const cellSize = Math.round(Number(App.$.zoom.value));
        App.dragPattern.renderCells(App.$.patternDragContainer, pattern, cellSize);

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
     * Draw a pattern's cells into a container at the grid's current cell size and colour
     * @param {HTMLElement} container
     * @param {import('./patterns').Pattern} pattern
     * @param {number} cellSize
     */
    renderCells(container, pattern, cellSize) {
      let maxX = -Infinity, maxY = -Infinity;
      for (const cell of pattern.cells) {
        maxX = Math.max(maxX, cell.x);
        maxY = Math.max(maxY, cell.y);
      }
      container.style.width = `${(maxX + 1) * cellSize}px`;
      container.style.height = `${(maxY + 1) * cellSize}px`;

      const fragment = document.createDocumentFragment();
      for (const cell of pattern.cells) {
        const cellDiv = document.createElement('div');
        cellDiv.style.cssText = `
          position: absolute;
          left: ${cell.x * cellSize}px;
          top: ${cell.y * cellSize}px;
          width: ${cellSize}px;
          height: ${cellSize}px;
          background-color: ${App.$.cellColour.value};
        `;
        fragment.appendChild(cellDiv);
      }
      container.replaceChildren(fragment);
    },
    /**
     * Place a pattern centred on a canvas pixel
     * @param {string} patternType
     * @param {number} originPx
     * @param {number} originPy
     */
    drop(patternType, originPx, originPy) {
      canvasWorkerMessage({
        type: CanvasWorkerMessageType.SetPattern,
        patternType,
        colour: App.cellColour.state(),
        originPx,
        originPy
      });
    },
    /** @type {{ active: boolean } | null} */
    touch: null,
    /**
     * Track a touch or pen press on a pattern. It becomes a drag after a sideways move or a
     * short hold; a vertical move is left to the browser so the list still scrolls.
     * @param {PointerEvent} down
     * @param {string} type
     * @param {HTMLElement} item
     */
    touchStart(down, type, item) {
      const pattern = /** @type {import('./patterns').Pattern} */ (Patterns.get(type));
      const ghost = App.$.patternGhost;
      const gesture = { active: false };
      let x = down.clientX, y = down.clientY, cellSize = 0;
      App.dragPattern.touch = gesture;
      item.draggable = false; // no native long-press drag on top of this one

      // The pattern is centred on the preview, which floats above the finger.
      const dropPoint = () => ({ x, y: y - TOUCH_DRAG_LIFT_PX });
      const overCanvas = (/** @type {{x: number, y: number}} */ p) =>
        document.elementsFromPoint(p.x, p.y).find((el) => !App.$.patternPanel.contains(el)) === App.$.canvas;
      const follow = () => {
        const p = dropPoint();
        ghost.style.translate = `${p.x - pattern.center_x * cellSize}px ${p.y - pattern.center_y * cellSize}px`;
        App.dragPattern.state.canvasDragOver = overCanvas(p);
      };
      const activate = () => {
        if (gesture.active) {
          return;
        }
        gesture.active = true;
        cellSize = Math.round(Number(App.$.zoom.value));
        App.dragPattern.renderCells(ghost, pattern, cellSize);
        ghost.hidden = false;
        App.$.patternPanel.dataset.dragging = '';
        App.dragPattern.state.type = type;
        follow();
      };
      const timer = setTimeout(activate, TOUCH_DRAG_HOLD_MS);

      const onMove = (/** @type {PointerEvent} */ e) => {
        if (e.pointerId !== down.pointerId) {
          return;
        }
        x = e.clientX;
        y = e.clientY;
        if (gesture.active) {
          follow();
          return;
        }
        const dx = x - down.clientX, dy = y - down.clientY;
        if (Math.hypot(dx, dy) > TOUCH_DRAG_SLOP_PX) {
          if (Math.abs(dx) > Math.abs(dy)) {
            activate();
          } else {
            finish();
          }
        }
      };
      const onUp = (/** @type {PointerEvent} */ e) => {
        if (e.pointerId !== down.pointerId) {
          return;
        }
        const p = dropPoint();
        if (gesture.active && overCanvas(p)) {
          const rect = App.$.canvas.getBoundingClientRect();
          App.dragPattern.drop(type, Math.round(p.x - rect.left), Math.round(p.y - rect.top));
        }
        finish();
      };
      const onCancel = (/** @type {PointerEvent} */ e) => {
        if (e.pointerId === down.pointerId) {
          finish();
        }
      };
      const finish = () => {
        clearTimeout(timer);
        item.removeEventListener('pointermove', onMove);
        item.removeEventListener('pointerup', onUp);
        item.removeEventListener('pointercancel', onCancel);
        item.draggable = true;
        ghost.hidden = true;
        ghost.replaceChildren();
        delete App.$.patternPanel.dataset.dragging;
        if (gesture.active) {
          App.dragPattern.end();
        }
        App.dragPattern.touch = null;
      };
      item.addEventListener('pointermove', onMove);
      item.addEventListener('pointerup', onUp);
      item.addEventListener('pointercancel', onCancel);
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
      // A press that light-dismisses the colour popover (it closes on release) only closes it.
      if (App.$.colourPopover.matches(':popover-open')) {
        return;
      }
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
  theme: {
    state: signal(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'),
    /**
     * @param {'light' | 'dark'} theme
     */
    apply(theme) {
      document.documentElement.dataset.theme = theme;
      App.theme.state.update(theme);
    }
  },
  zoom: {
    /**
     * Sync the zoom control with a cell size in pixels
     * @param {number} cellSize
     */
    set(cellSize) {
      App.$.zoom.value = String(cellSize);
      App.$.zoomLabel.textContent = `${cellSize}×`;
      App.$.zoom.setAttribute('aria-valuetext', `Zoom ${cellSize}×`);
      // Light whole squares only: the track is 8 squares of 10px with 4px gaps (108px).
      const lit = Math.max(1, Math.ceil(((cellSize - MIN_ZOOM) / (MAX_ZOOM - MIN_ZOOM)) * 8));
      App.$.zoom.style.setProperty('--fill', `${(lit * 14 / 108) * 100}%`);
    }
  },
  speed: {
    state: signal(0),
    /**
     * Set the speed input to a slider value and notify listeners
     * @param {number} value
     */
    setValue(value) {
      const previous = App.$.speed.value;
      App.$.speed.value = String(Math.max(SPEED_MIN, Math.min(SPEED_MAX, value)));
      if (App.$.speed.value !== previous) {
        App.$.speed.dispatchEvent(new Event('input', { bubbles: true }));
      }
    },
    /**
     * Turn the dial towards the pointer. The dial sweeps 270° with the gap at the bottom.
     * @param {PointerEvent} e
     */
    setFromPointer(e) {
      const rect = App.$.speedDial.getBoundingClientRect();
      const dx = e.clientX - (rect.left + rect.width / 2);
      const dy = e.clientY - (rect.top + rect.height / 2);
      const halfSweep = DIAL_SWEEP_DEG / 2;
      let angle = Math.atan2(dx, -dy) * 180 / Math.PI;
      if (Math.abs(angle) > halfSweep) {
        // In the gap: stay at the end the dial is closest to instead of jumping across.
        angle = Number(App.$.speed.value) > (SPEED_MIN + SPEED_MAX) / 2 ? halfSweep : -halfSweep;
      }
      App.speed.setValue(SPEED_MIN + ((angle + halfSweep) / DIAL_SWEEP_DEG) * (SPEED_MAX - SPEED_MIN));
    },
    /**
     * Draw the dial's notches and pointer for the current input value on a 23x23 pixel grid
     */
    renderDial() {
      const t = (Number(App.$.speed.value) - SPEED_MIN) / (SPEED_MAX - SPEED_MIN);
      const angle = t * DIAL_SWEEP_DEG - DIAL_SWEEP_DEG / 2;
      /**
       * @param {number} x
       * @param {number} y
       */
      const pixel = (x, y) => `M${x} ${y}h1v1h-1z`;

      let arc = '', lit = '';
      for (const p of DIAL_ARC) {
        if (p.angle <= angle + 1e-9) {
          lit += pixel(p.x, p.y);
        } else {
          arc += pixel(p.x, p.y);
        }
      }

      // Bresenham line from the centre pixel to the pointer tip.
      const rad = angle * Math.PI / 180;
      const x1 = Math.round(DIAL_CENTRE + DIAL_POINTER_LENGTH * Math.sin(rad));
      const y1 = Math.round(DIAL_CENTRE - DIAL_POINTER_LENGTH * Math.cos(rad));
      const dx = Math.abs(x1 - DIAL_CENTRE), sx = x1 > DIAL_CENTRE ? 1 : -1;
      const dy = -Math.abs(y1 - DIAL_CENTRE), sy = y1 > DIAL_CENTRE ? 1 : -1;
      let x = DIAL_CENTRE, y = DIAL_CENTRE, err = dx + dy, pointer = '';
      for (;;) {
        pointer += pixel(x, y);
        if (x === x1 && y === y1) {
          break;
        }
        const e2 = 2 * err;
        if (e2 >= dy) {
          err += dy;
          x += sx;
        }
        if (e2 <= dx) {
          err += dx;
          y += sy;
        }
      }

      App.$.speedArc.setAttribute('d', arc);
      App.$.speedArcLit.setAttribute('d', lit);
      App.$.speedPointer.setAttribute('d', pointer);
    },
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
