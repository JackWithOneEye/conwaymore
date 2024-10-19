package livereload

import "html/template"

type livereloadScriptConfig struct {
	Path          string
	RetryInterval uint
	MaxRetries    uint
}

const livereloadScript = `
  const socketUrl = "ws://" + location.host + "/{{.Path}}";
  const retryInterval = {{.RetryInterval}};
  const maxRetries = {{.MaxRetries}};
  const ws = new WebSocket(socketUrl);
  ws.onopen = () => {
    console.log("Livereload connection open.");
  };
  ws.onclose = () => {
    console.log("Livereload connection closed.");  
    let retries = 0;
    const reload = () => {
      retries++;
      if (retries > maxRetries) {
        console.error("Could not reconnect to server.");
        return;
      }

      const ws2 = new WebSocket(socketUrl);
      ws2.onerror = () => {
        setTimeout(reload, retryInterval);
      };
      ws2.onopen = () => {
        location.reload();
      };
    };
    reload();
  };
  ws.onerror = (e) => {
    console.error("Livereload connection error", e);
  };
`

var livereloadScriptTemplate = template.Must(template.New("livereloadScript").Parse(livereloadScript))
