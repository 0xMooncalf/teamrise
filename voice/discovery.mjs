// @discordjs/voice 0.19.2 sends IP discovery once. Retry the same standard
// packet on loss; the library still owns the reply, protocol selection and DAVE.
export function retryIPDiscovery(connection, { intervalMs = 1000, limit = 7, onRetry = () => {} } = {}) {
  let attempts = 0;
  const timer = setInterval(() => {
    if (connection.state.status === 'destroyed' || connection.state.status === 'ready') { clearInterval(timer); return; }
    const network = connection.state.networking?.state;
    if (network?.code !== 2 || !network.udp || !Number.isInteger(network.connectionData?.ssrc)) return;
    if (attempts >= limit) { clearInterval(timer); return; }
    const packet = Buffer.alloc(74);
    packet.writeUInt16BE(1, 0); packet.writeUInt16BE(70, 2);
    packet.writeUInt32BE(network.connectionData.ssrc, 4);
    try { network.udp.send(packet); onRetry(++attempts); }
    catch { clearInterval(timer); } // The transport can close while a retry is queued.
  }, intervalMs);
  timer.unref?.();
  return () => clearInterval(timer);
}
