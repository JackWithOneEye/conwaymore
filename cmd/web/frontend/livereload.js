const socketUrl = `ws://${location.host}/_livereload`
const ws = new WebSocket(socketUrl);
ws.onopen = () => {
    console.log("Livereload connection open.");
};
ws.onclose = () => {
    console.log("Livereload connection closed.");
    const retryInterval = 500;
    const maxRetries = 10;
    let retries = 0;
    const reload = () => {
        retries++;
        if (retries > maxRetries) {
            console.error("Could not reconnect to dev server.");
            return;
        }

        const wsRecover = new WebSocket(socketUrl);
        wsRecover.onerror = () => {
            setTimeout(reload, retryInterval);
        };
        wsRecover.onopen = () => {
            location.reload();
        };
    };
    reload();
};
ws.onerror = (/** @type {unknown} */ e) => {
    console.error("Livereload connection error", e);
};