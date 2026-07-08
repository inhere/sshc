import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";

export type TerminalMount = {
  terminal: Terminal;
  dispose: () => void;
};

export function mountTerminal(
  container: HTMLElement,
  sessionID: string,
  onResize: (cols: number, rows: number) => void,
  onClose?: () => void,
): TerminalMount {
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

  let disposed = false;
  const socket = new WebSocket(terminalWSURL(sessionID));
  socket.binaryType = "arraybuffer";
  const sendToPTY = (data: string) => {
    if (socket.readyState === WebSocket.OPEN) {
      socket.send(data);
    }
  };
  const fitTerminal = () => {
    if (disposed || container.clientWidth === 0 || container.clientHeight === 0) return;
    fit.fit();
  };
  const fitAndResize = () => {
    fitTerminal();
    onResize(terminal.cols, terminal.rows);
  };
  const queueFitAndResize = () => {
    window.requestAnimationFrame(() => {
      fitAndResize();
      window.setTimeout(fitAndResize, 50);
    });
  };
  terminal.attachCustomKeyEventHandler((event) => {
    if (event.type !== "keydown") return true;
    const shortcut = (event.ctrlKey || event.metaKey) && !event.altKey;
    const key = event.key.toLowerCase();
    if (shortcut && key === "c") {
      if (terminal.hasSelection()) {
        if (!navigator.clipboard?.writeText) return true;
        void writeClipboardText(terminal.getSelection());
      } else {
        sendToPTY("\x03");
      }
      event.preventDefault();
      return false;
    }
    if (shortcut && key === "v") {
      return true;
    }
    if (event.key === "Escape") {
      sendToPTY("\x1b");
      event.preventDefault();
      return false;
    }
    return true;
  });
  const pasteHandler = (event: ClipboardEvent) => {
    const text = event.clipboardData?.getData("text/plain") || "";
    if (!text) return;
    event.preventDefault();
    event.stopPropagation();
    sendToPTY(text);
  };
  const focusHandler = () => terminal.focus();
  container.addEventListener("paste", pasteHandler, true);
  container.addEventListener("pointerdown", focusHandler);
  queueFitAndResize();
  void document.fonts?.ready.then(queueFitAndResize);
  socket.addEventListener("open", () => {
    fitAndResize();
    queueFitAndResize();
    terminal.focus();
  });
  socket.addEventListener("message", (event) => {
    if (event.data instanceof ArrayBuffer) {
      terminal.write(new Uint8Array(event.data));
      return;
    }
    terminal.write(String(event.data));
  });
  socket.addEventListener("close", () => {
    if (!disposed) {
      terminal.writeln("\r\n[disconnected]");
      onClose?.();
    }
  });
  socket.addEventListener("error", () => {
    if (!disposed) terminal.writeln("\r\n[connection error]");
  });

  const dataDisposable = terminal.onData((data) => {
    sendToPTY(data);
  });
  const resizeHandler = () => {
    fitAndResize();
  };
  window.addEventListener("resize", resizeHandler);
  return {
    terminal,
    dispose: () => {
      disposed = true;
      dataDisposable.dispose();
      window.removeEventListener("resize", resizeHandler);
      container.removeEventListener("paste", pasteHandler, true);
      container.removeEventListener("pointerdown", focusHandler);
      socket.close();
      terminal.dispose();
    },
  };
}

function terminalWSURL(sessionID: string) {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/api/terminal/sessions/${encodeURIComponent(sessionID)}/ws`;
}

async function writeClipboardText(text: string) {
  try {
    await navigator.clipboard?.writeText(text);
  } catch {
    // Ignore clipboard permission failures; terminal interrupt/paste handling still works.
  }
}
