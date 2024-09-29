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

