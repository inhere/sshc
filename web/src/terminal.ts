import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";

export type TerminalMount = {
  terminal: Terminal;
  dispose: () => void;
};

export function mountTerminal(container: HTMLElement, sessionID: string, onResize: (cols: number, rows: number) => void): TerminalMount {
  const terminal = new Terminal({
    cursorBlink: true,
    fontFamily: '"Cascadia Mono", "SFMono-Regular", Consolas, monospace',
    fontSize: 13,
    convertEol: true,
    theme: {
      background: "#111827",
      foreground: "#dbeafe",
      cursor: "#f8fafc",
      selectionBackground: "#334155",
    },
  });
  const fit = new FitAddon();
  terminal.loadAddon(fit);
  terminal.open(container);
  fit.fit();

  const socket = new WebSocket(terminalWSURL(sessionID));
  socket.binaryType = "arraybuffer";
  socket.addEventListener("open", () => {
    fit.fit();
    onResize(terminal.cols, terminal.rows);
  });
  socket.addEventListener("message", (event) => {
    if (event.data instanceof ArrayBuffer) {
      terminal.write(new Uint8Array(event.data));
      return;
    }
    terminal.write(String(event.data));
  });
  socket.addEventListener("close", () => terminal.writeln("\r\n[disconnected]"));
  socket.addEventListener("error", () => terminal.writeln("\r\n[connection error]"));

  const dataDisposable = terminal.onData((data) => {
    if (socket.readyState === WebSocket.OPEN) {
      socket.send(data);
    }
  });
  const resizeHandler = () => {
    fit.fit();
    onResize(terminal.cols, terminal.rows);
  };
  window.addEventListener("resize", resizeHandler);
  return {
    terminal,
    dispose: () => {
      dataDisposable.dispose();
      window.removeEventListener("resize", resizeHandler);
      socket.close();
      terminal.dispose();
    },
  };
}

function terminalWSURL(sessionID: string) {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/api/terminal/sessions/${encodeURIComponent(sessionID)}/ws`;
}
