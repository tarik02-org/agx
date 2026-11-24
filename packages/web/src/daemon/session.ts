// The UI's session with the daemon: a token /ui/login hands over in the address fragment, kept
// in this origin's storage and sent as a bearer token. Not a cookie, because browsers send a
// host's cookies to every port on it, so other local servers would receive one.

const storageKey = "agx.session";

/** Keeps the session /ui/login handed over, if any, and drops it from the address. */
export const takeSession = (): void => {
  const handed = new URLSearchParams(globalThis.location.hash.slice(1)).get("session");
  if (handed === null || handed === "") {
    return;
  }
  globalThis.localStorage.setItem(storageKey, handed);
  const { pathname, search } = globalThis.location;
  globalThis.history.replaceState(globalThis.history.state, "", pathname + search);
};

/** The session token, or empty before any sign-in in this browser. */
export const sessionToken = (): string => globalThis.localStorage.getItem(storageKey) ?? "";

/** Headers that authorize a request to the daemon. */
export const sessionHeaders = (): Record<string, string> => {
  const token = sessionToken();
  return token === "" ? {} : { Authorization: `Bearer ${token}` };
};
