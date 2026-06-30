/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: false, // canvas/interval effects double-fire under StrictMode dev; off for clean animation loops
};
export default nextConfig;
