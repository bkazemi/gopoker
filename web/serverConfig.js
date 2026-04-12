const config = {
  gopokerServerAddr: process.env.NEXT_PUBLIC_GOPOKER_SERVER_ADDR || 'localhost',
  gopokerServerHTTPURL: process.env.NEXT_PUBLIC_GOPOKER_SERVER_HTTPURL,
  gopokerServerWSURL: process.env.NEXT_PUBLIC_GOPOKER_SERVER_WSURL,

  sslEnabled: (process.env.NEXT_PUBLIC_SSL_ENABLED || '').toLowerCase() === 'true',
};

if (!config.gopokerServerHTTPURL)
  config.gopokerServerHTTPURL = (config.sslEnabled ? 'https://' : 'http://') + config.gopokerServerAddr;

if (!config.gopokerServerWSURL)
  config.gopokerServerWSURL = (config.sslEnabled ? 'wss://' : 'ws://') + config.gopokerServerAddr;

export default config;
