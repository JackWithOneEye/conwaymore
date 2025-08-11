/**
 * @param {string} id 
 * @returns {HTMLElement}
 */
export function getElementByIdOrDie(id) {
  const el = document.getElementById(id);
  if (!el) {
    throw Error(`element #${id} not found`)
  }
  return el;
}

/**
 * Display an error message to the user in a dismissible banner
 * @param {string} message - The error message to display
 */
export function showError(message) {
  // Create error banner if it doesn't exist
  const errorBanner = getElementByIdOrDie('error-banner');
  errorBanner.textContent = message;
  errorBanner.classList.remove('-translate-y-full');

  // Auto-hide after 5 seconds
  setTimeout(() => {
    errorBanner.classList.add('-translate-y-full');
  }, 5000);
}

/**
 * @typedef {Partial<{
 *   cellSize: number,
 *   speed: number,
 *   cellColour: number,
 *   showAge: boolean,
 *   drawGrid: boolean
 * }>} UserPrefs
 */

/**
 * LocalStorage persistence utility for user preferences
 */
export const UserPreferences = {
  /** @type {string} */
  STORAGE_KEY: 'conwaymore-prefs',

  /**
   * @param {UserPrefs} prefs - The preferences to save
   */
  save(prefs) {
    try {
      const existing = this.load();
      const updated = { ...existing, ...prefs };
      localStorage.setItem(this.STORAGE_KEY, JSON.stringify(updated));
    } catch (error) {
      console.warn('Failed to save preferences:', error);
    }
  },

  /**
   * @returns {Partial<UserPrefs>} - The loaded preferences or empty object if none found
   */
  load() {
    try {
      const stored = localStorage.getItem(this.STORAGE_KEY);
      return stored ? JSON.parse(stored) : {};
    } catch (error) {
      console.warn('Failed to load preferences:', error);
      return {};
    }
  }
};

