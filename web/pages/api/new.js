import fetch from 'node-fetch';

import config from '@/serverConfig';

export default async function handler(req, res) {
  if (req.method !== 'POST') {
    res.status(405).end();
    return;
  }

  try {
    console.log(`/api/new: req.body: ${JSON.stringify(req.body)}`);
    const srvRes = await fetch(`${config.gopokerServerHTTPURL}/new`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(req.body),
    });

    if (!srvRes.ok)
      throw new Error('request failed');

    res.status(200).send(await srvRes.text());
  } catch (err) {
    console.log(`couldn't POST to external server's /new: ${err}`);
    res.status(500).json({ error: err.message });
  }
}
