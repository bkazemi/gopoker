import fetch from 'node-fetch';

import config from '@/serverConfig';

export default async function handler(req, res) {
  const { roomID } = req.query;

  if (req.method !== 'GET') {
    res.status(405).end();
    return;
  }

  try {
    console.log(`/api/check: roomID: ${roomID}`);
    const srvRes = await fetch(`${config.gopokerServerHTTPURL}/room/${roomID}`);

    if (!srvRes.ok) {
      console.log('status', srvRes.status, typeof(srvRes.status));
      if (srvRes.status === 404)
        res.status(404).send();
      else if (srvRes.status === 403)
        res.status(403).send();
      else
        throw new Error(`request failed (HTTP status: ${srvRes.status})`);
    } else
      res.status(200).send(await srvRes.text());
  } catch (err) {
    console.log(`couldn't GET on external server's /room: ${err}`);
    res.status(500).json({ error: err.message });
  }
}
