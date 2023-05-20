import fetch from 'node-fetch';

export default async function handler(req, res) {
  if (req.method !== 'GET') {
    res.status(405).end();
    return;
  }

  try {
    const srvRes = await fetch('http://10.0.1.2:7755/status');

    if (!srvRes.ok)
      throw new Error('request failed');

    res.status(200).send(await srvRes.text());
  } catch (err) {
    console.log(`couldn't GET to external server's /roomCount: ${err}`);
    res.status(500).json({ error: err.message });
  }
}
