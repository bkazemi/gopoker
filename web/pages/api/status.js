import fetch from 'node-fetch';

const srvURL = 'https://gopoker-server.onrender.com';

export default async function handler(req, res) {
  if (req.method !== 'GET') {
    res.status(405).end();
    return;
  }

  try {
    const srvRes = await fetch(`${srvURL}/status`);

    if (!srvRes.ok)
      throw new Error('request failed');

    res.status(200).send(await srvRes.text());
  } catch (err) {
    console.log(`couldn't GET to external server's /roomCount: ${err}`);
    res.status(500).json({ error: err.message });
  }
}
