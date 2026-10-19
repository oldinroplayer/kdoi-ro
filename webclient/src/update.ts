// Server restarts without logging players out: whenever it shuts down cleanly
// (an update into a new build, or Ctrl-C before starting it again) the server
// sends ServerUpdating (a wait and a one-time resume token)
// and closes the connection with CLOSE_UPDATING. The page keeps the token in
// sessionStorage, shows "updating", polls the server until the new one
// answers, then reloads (picking up the new client too) and logs back in with
// the token instead of the password.

const STORAGE_KEY = "kdoiro.resume";
/** How often to check whether the new server is up (ms). */
const POLL_MS = 1500;

/** What the page needs to log back in after reloading. */
export interface ResumeInfo {
  token: string;
  name: string;
  /** The server's WebSocket URL. */
  server: string;
  /** Auto attack was running: turn it back on. */
  bot: boolean;
  /** Date.now() after which the token is no good anyway. */
  until: number;
}

export function saveResume(info: ResumeInfo): void {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(info));
  } catch {
    // Storage blocked: after the update the player logs in by hand.
  }
}

/** The saved resume, if any and not yet expired. */
export function loadResume(): ResumeInfo | null {
  try {
    const info = JSON.parse(sessionStorage.getItem(STORAGE_KEY) ?? "null") as ResumeInfo | null;
    if (info && typeof info.token === "string" && info.until > Date.now()) return info;
  } catch {
    // Unreadable: treat as none.
  }
  clearResume();
  return null;
}

export function clearResume(): void {
  try {
    sessionStorage.removeItem(STORAGE_KEY);
  } catch {
    // Nothing to clear.
  }
}

/** The server's health check URL, from its WebSocket URL. */
function healthUrl(wsUrl: string): string {
  const url = new URL(wsUrl);
  url.protocol = url.protocol === "wss:" ? "https:" : "http:";
  url.pathname = "/healthz";
  url.search = "";
  return url.toString();
}

/**
 * Polls the server until it answers (then calls onUp) or `seconds` pass (then
 * onTimeout). Returns a function that stops waiting.
 */
export function waitForServer(wsUrl: string, seconds: number, onUp: () => void, onTimeout: () => void): () => void {
  const deadline = Date.now() + seconds * 1000;
  let stopped = false;
  let timer = 0;
  const check = async () => {
    if (stopped) return;
    try {
      // no-cors: the dev client runs on another port; any answer at all
      // (even an opaque one) means the server is listening again.
      await fetch(healthUrl(wsUrl), { cache: "no-store", mode: "no-cors" });
      if (!stopped) {
        stopped = true;
        onUp();
        return;
      }
    } catch {
      // Not up yet.
    }
    if (stopped) return;
    if (Date.now() >= deadline) {
      stopped = true;
      onTimeout();
      return;
    }
    timer = window.setTimeout(check, POLL_MS);
  };
  // The old server is still closing its connections: give it a moment.
  timer = window.setTimeout(check, POLL_MS);
  return () => {
    stopped = true;
    clearTimeout(timer);
  };
}

/** The full-screen "server restarting" notice, with Retry / Log in buttons once the wait runs out. */
export class UpdateOverlay {
  private readonly root = document.createElement("div");
  private readonly title = document.createElement("h2");
  private readonly text = document.createElement("p");
  private readonly actions = document.createElement("div");
  private countdown = 0;

  constructor() {
    this.root.className = "update-overlay";
    this.root.hidden = true;
    this.root.setAttribute("role", "alertdialog");
    this.root.setAttribute("aria-live", "assertive");
    const box = document.createElement("div");
    box.className = "update-box";
    const spinner = document.createElement("div");
    spinner.className = "update-spinner";
    this.actions.className = "update-actions";
    box.append(spinner, this.title, this.text, this.actions);
    this.root.append(box);
    document.body.append(this.root);
  }

  /** "Restarting": counts down the seconds the server asked us to wait. */
  waiting(seconds: number): void {
    this.open("Server restarting", "");
    this.root.classList.remove("stuck");
    const end = Date.now() + seconds * 1000;
    const tick = () => {
      const left = Math.max(0, Math.ceil((end - Date.now()) / 1000));
      this.text.textContent = `The server is restarting, maybe with a new version. You'll be back where you were in a moment… (${left}s)`;
    };
    tick();
    clearInterval(this.countdown);
    this.countdown = window.setInterval(tick, 500);
  }

  /** Logging back in after the reload. */
  rejoining(): void {
    clearInterval(this.countdown);
    this.root.classList.remove("stuck");
    this.open("Rejoining", "Logging you back in…");
  }

  /** The wait ran out: let the player keep trying or log in by hand. */
  stuck(onRetry: () => void, onLogin: () => void): void {
    clearInterval(this.countdown);
    this.open("The server isn't back yet", "It's taking longer than expected. You can keep waiting or log in again later.");
    this.root.classList.add("stuck");
    const retry = document.createElement("button");
    retry.type = "button";
    retry.textContent = "Keep waiting";
    retry.addEventListener("click", onRetry);
    const login = document.createElement("button");
    login.type = "button";
    login.className = "secondary";
    login.textContent = "Back to login";
    login.addEventListener("click", onLogin);
    this.actions.append(login, retry);
    retry.focus();
  }

  hide(): void {
    clearInterval(this.countdown);
    this.root.hidden = true;
  }

  private open(title: string, text: string): void {
    this.title.textContent = title;
    this.text.textContent = text;
    this.actions.replaceChildren();
    this.root.hidden = false;
  }
}
