export default function handler(req, res) {
  const socket = new WebSocket('ws://0.0.0.0:777/web');

  socket.on('open', () => {
    console.log('ws: ws opened');
  });

  socket.on('message', (data) => {
    console.log('ws: recv msg');
  });
}
