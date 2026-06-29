export function websocketHostIsAllowed(
  currentHostname: string,
  configuredDomain: string,
): boolean {
  if (
    configuredDomain === "localhost"
    && remoteTunnelHostIsAllowed(currentHostname)
  ) {
    return true;
  }
  return currentHostname === configuredDomain;
}

function remoteTunnelHostIsAllowed(currentHostname: string): boolean {
  return (
    currentHostname.endsWith(".trycloudflare.com")
    || currentHostname.endsWith(".localhost.run")
    || currentHostname.endsWith(".lhr.life")
  );
}
