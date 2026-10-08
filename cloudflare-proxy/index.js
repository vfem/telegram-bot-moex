let cachedIdToken = null;
let tokenExpiresAt = 0;
let importedPrivateKey = null;

function base64url(buffer) {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary)
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replaceAll("=", "");
}

function strBase64url(str) {
  return btoa(str)
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replaceAll("=", "");
}

async function getPrivateKey(pem) {
  if (importedPrivateKey) return importedPrivateKey;
  const pemHeader = "-----BEGIN PRIVATE KEY-----";
  const pemFooter = "-----END PRIVATE KEY-----";
  const pemContents = pem
    .substring(pem.indexOf(pemHeader) + pemHeader.length, pem.indexOf(pemFooter))
    .replace(/\s+/g, "");
  const binaryDer = Uint8Array.from(atob(pemContents), c => c.charCodeAt(0));

  importedPrivateKey = await crypto.subtle.importKey(
    "pkcs8",
    binaryDer,
    { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
    false,
    ["sign"]
  );
  return importedPrivateKey;
}

async function getGoogleIdToken(env) {
  const now = Math.floor(Date.now() / 1000);
  if (cachedIdToken && tokenExpiresAt > now + 60) {
    return cachedIdToken;
  }

  const key = await getPrivateKey(env.PRIVATE_KEY_PEM);

  const header = { alg: "RS256", typ: "JWT", kid: "cf-key-1" };
  const payload = {
    iss: env.ISSUER_URI,
    sub: "cf-worker",
    aud: env.WIF_AUDIENCE,
    iat: now,
    exp: now + 300,
  };

  const unsigned = strBase64url(JSON.stringify(header)) + "." + strBase64url(JSON.stringify(payload));
  const enc = new TextEncoder();
  const signature = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", key, enc.encode(unsigned));
  const jwt = unsigned + "." + base64url(signature);

  // 1. Exchange JWT with Google STS
  const stsRes = await fetch("https://sts.googleapis.com/v1/token", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      grant_type: "urn:ietf:params:oauth:grant-type:token-exchange",
      audience: env.WIF_PROVIDER,
      scope: "https://www.googleapis.com/auth/cloud-platform",
      requested_token_type: "urn:ietf:params:oauth:token-type:access_token",
      subject_token_type: "urn:ietf:params:oauth:token-type:id_token",
      subject_token: jwt,
    }),
  });

  const stsData = await stsRes.json();
  if (!stsData.access_token) {
    throw new Error("STS exchange failed: " + JSON.stringify(stsData));
  }

  // 2. Request Google ID Token for Cloud Run
  const idRes = await fetch(
    `https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/${env.SERVICE_ACCOUNT_EMAIL}:generateIdToken`,
    {
      method: "POST",
      headers: {
        Authorization: "Bearer " + stsData.access_token,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        audience: env.CLOUD_RUN_URL,
        includeEmail: true,
      }),
    }
  );

  const idData = await idRes.json();
  if (!idData.token) {
    throw new Error("generateIdToken failed: " + JSON.stringify(idData));
  }

  cachedIdToken = idData.token;
  tokenExpiresAt = now + 3300; // cache for 55 minutes
  return cachedIdToken;
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (request.method === "GET" && (url.pathname === "/health" || url.pathname === "/healthz")) {
      return new Response(JSON.stringify({ status: "healthy", service: "cf-edge-proxy" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }

    if (request.method !== "POST") {
      return new Response("Method not allowed", { status: 405 });
    }

    // 1. Verify Telegram secret token at Cloudflare edge
    const secret = request.headers.get("X-Telegram-Bot-Api-Secret-Token");
    if (!secret || secret !== env.TELEGRAM_SECRET_TOKEN) {
      return new Response("Unauthorized", { status: 401 });
    }

    try {
      // 2. Fetch or retrieve cached Google IAM ID Token
      const idToken = await getGoogleIdToken(env);

      // 3. Forward to private Cloud Run with IAM Bearer token
      const url = new URL(request.url);
      const targetUrl = env.CLOUD_RUN_URL + url.pathname;

      const newHeaders = new Headers(request.headers);
      newHeaders.set("Authorization", "Bearer " + idToken);

      const bodyText = await request.text();

      return await fetch(targetUrl, {
        method: request.method,
        headers: newHeaders,
        body: bodyText,
      });
    } catch (err) {
      return new Response("Edge Auth Error: " + err.message, { status: 502 });
    }
  },
};
