const config = {
  gopokerServerAddr: process.env.NEXT_PUBLIC_GOPOKER_SERVER_ADDR || 'localhost',
  gopokerServerHTTPURL: '',
  gopokerServerWSURL: '',

  sslEnabled: process.env.NEXT_PUBLIC_SSL_ENABLED.toLowerCase() === 'true',
};

config.gopokerServerHTTPURL = (config.sslEnabled ? 'https://' : 'http://') + config.gopokerServerAddr;
config.gopokerServerWSURL = (config.sslEnabled ? 'wss://' : 'ws://') + config.gopokerServerAddr;

export default config;
