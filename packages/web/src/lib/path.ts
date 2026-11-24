/** Shows a path under the home directory as ~/…; other paths stay as they are. */
export const shortPath = (path: string, home: string): string =>
  home !== "" && (path === home || path.startsWith(`${home}/`))
    ? `~${path.slice(home.length)}`
    : path;
