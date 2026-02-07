/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
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
