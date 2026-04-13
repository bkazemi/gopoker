const path = require('path');

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  turbopack: {
    root: path.join(__dirname),
  },
  async redirects() {
    return [
      {
        source: '/room',
        destination: '/rooms',
        permanent: false,
      },
    ];
  },
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          {
            key: "Content-Security-Policy",
            value: "frame-ancestors 'self' https://b.shirkadeh.org https://shirkadeh.org https://bkazemi.github.io",
          },
        ],
      },
    ];
  },
}

module.exports = nextConfig
