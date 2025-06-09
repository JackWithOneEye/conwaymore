/** @import { EffectInstance, Signal, UnlinkFn } from './types/signals' */

/**
 * @template T
 * @param {T} value 
 * @returns {Signal<T>}
 */
export function signal(value) {
  let refObj = { value };

  const signalInstance = () => {
    track(refObj, 'value');
    return refObj.value;
  };

  /**
   * @param {T | ((curr: T) => T)} valueOrFn 
   */
  signalInstance.update = (valueOrFn) => {
    // @ts-ignore
    const newValue = typeof valueOrFn === 'function' ? valueOrFn(internalValue) : valueOrFn

    if (newValue === refObj.value) {
      return;
    }
    refObj.value = newValue;
    notify(refObj, 'value');
  };

  return signalInstance;
}

/**
 * @template {object} T
 * @param {T} obj 
 * @returns {T}
 */
export function reactive(obj) {
  return new Proxy(obj, {
    get(target, key) {
      track(target, key);
      // @ts-ignore
      return target[key];
    },
    set(target, key, newValue) {
      // @ts-ignore
      if (target[key] === newValue) {
        return false;
      }
      // @ts-ignore
      target[key] = newValue;
      notify(target, key);
      return true;
    }
  });
}

/**
 * @template T
 * @param {() => T} fn 
 */
export function computed(fn) {
  /** @type {Set<UnlinkFn>} */
  const unlinks = new Set();

  /** @type {{ value: T }} */
  // @ts-ignore
  const refObj = {};

  const signalInstance = () => {
    track(refObj, 'value');
    return refObj.value;
  };

  /** @type {EffectInstance} */
  const effectInstance = {
    execute: () => {
      activeEffect = effectInstance;
      const newComputedValue = fn();
      activeEffect = null;
      if (newComputedValue !== refObj.value) {
        refObj.value = newComputedValue;
        notify(refObj, 'value');
      }
    },
    link: (unlink) => {
      unlinks.add(unlink);
    }
  };
  effectInstance.execute();

  return signalInstance;
}

/**
 * @param {() => (() => void) | void} fn 
 * @returns {() => void}
 */
export function effect(fn) {
  /** @type {(() => void) | undefined} */
  let cleanup;

  /** @type {Set<UnlinkFn>} */
  const unlinks = new Set();

  /** @type {EffectInstance} */
  const effectInstance = {
    execute: () => {
      activeEffect = effectInstance;
      // @ts-ignore
      cleanup = fn();
      activeEffect = null;
    },
    link: (unlink) => {
      unlinks.add(unlink);
    }
  };

  function dispose() {
    for (const unlink of unlinks) {
      unlink(effectInstance);
    }
    unlinks.clear();

    if (typeof cleanup === 'function') {
      cleanup();
    }
  }
  effectInstance.execute();

  return dispose;
}

/** @type {EffectInstance | null} */
let activeEffect = null

/** @type {WeakMap<object, Map<string | symbol, Set<EffectInstance>>>} */
const subscriptions = new WeakMap();

/**
 * @param {object} target 
 * @param {string | symbol} key 
 */
function track(target, key) {
  if (!activeEffect) {
    return;
  }
  let subs = subscriptions.get(target);
  if (!subs) {
    subs = new Map();
    subscriptions.set(target, subs);
  }
  let effects = subs.get(key);
  if (!effects) {
    effects = new Set();
    subs.set(key, effects);
  }
  if (!effects.has(activeEffect)) {
    effects.add(activeEffect);
    activeEffect.link((effect) => effects.delete(effect));
  }
}

/**
 * @param {object} target 
 * @param {string | symbol} key 
 */
function notify(target, key) {
  const effects = subscriptions.get(target)?.get(key);
  if (!effects) {
    return;
  }
  for (const e of effects) {
    e.execute();
  }
}
