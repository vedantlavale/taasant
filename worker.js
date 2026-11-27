// The site is static files in ./site. This one route serves the Mac app from
// the site's own address so a click starts the download of the newest DMG
// straight away, instead of sending people to a GitHub page that 404s when a
// release is built without the app.
//
// GitHub keeps the "latest" pointer itself: releases/latest/download/<name>
// is a redirect to the file on the newest release, so this never needs editing
// when a version comes out. Everything else falls through to the static site.
const LATEST_DMG =
  "https://github.com/vedantlavale/taasant/releases/latest/download/Taasant.dmg";

export default {
  async fetch(request, env) {
    const { pathname } = new URL(request.url);
    if (pathname === "/download/mac" || pathname === "/download/mac/") {
      return downloadMac();
    }
    return env.ASSETS.fetch(request);
  },
};

async function downloadMac() {
  let upstream;
  try {
    upstream = await fetch(LATEST_DMG, {
      redirect: "follow",
      headers: { "User-Agent": "taasant-site" },
    });
  } catch {
    upstream = null;
  }
  if (!upstream || !upstream.ok || !upstream.body) {
    return new Response(
      "The Mac app is not available right now. Please try again in a moment.\n",
      {
        status: 502,
        headers: {
          "Content-Type": "text/plain; charset=utf-8",
          "Cache-Control": "no-store",
        },
      },
    );
  }
  return new Response(upstream.body, {
    headers: {
      "Content-Type": "application/x-apple-diskimage",
      "Content-Disposition": 'attachment; filename="Taasant.dmg"',
      "Cache-Control": "public, max-age=300",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
