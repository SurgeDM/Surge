// Surge Download Manager - Background Service Worker
// Intercepts downloads and sends them to local Surge instance

const DEFAULT_PORT = 1700;
const MAX_PORT_SCAN = 100;
const INTERCEPT_ENABLED_KEY = "interceptEnabled";
const AUTH_TOKEN_KEY = "authToken";
const AUTH_VERIFIED_KEY = "authVerified";
const SERVER_URL_KEY = "serverUrl";
const MB = 1 << 20;

// === State ===
let cachedPort = null;
let cachedServerUrl = null;
let downloads = new Map();
let lastHealthCheck = 0;
let isConnected = false;
let cachedAuthToken = null;

// Pending duplicate downloads waiting for user confirmation
// Key: unique id, Value: { downloadItem, filename, directory, timestamp }
const pendingDuplicates = new Map();
let pendingDuplicateCounter = 0;

function updateBadge() {
  const count = pendingDuplicates.size;
  if (count > 0) {
    chrome.action.setBadgeText({ text: count.toString() });
    chrome.action.setBadgeBackgroundColor({ color: "#FF0000" }); // Red
  } else {
    chrome.action.setBadgeText({ text: "" });
  }
}

// === Header Capture ===
// Store request headers for URLs to forward to Surge (cookies, auth, etc.)
// Key: URL, Value: { headers: {}, timestamp: Date.now() }
const capturedHeaders = new Map();
const HEADER_EXPIRY_MS = 120000; // 2 minutes - headers expire after this time

// Capture all headers from requests using webRequest API
chrome.webRequest.onBeforeSendHeaders.addListener(
  (details) => {
    if (!details.requestHeaders || !details.url) return;

    // Capture all headers
    const headers = {};
    for (const header of details.requestHeaders) {
      headers[header.name] = header.value;
    }

    // Only store if we captured something
    if (Object.keys(headers).length > 0) {
      capturedHeaders.set(details.url, {
        headers,
        timestamp: Date.now(),
      });

      // Cleanup old entries periodically
      if (capturedHeaders.size > 1000) {
        cleanupExpiredHeaders();
      }
    }
  },
  { urls: ["<all_urls>"] },
  ["requestHeaders", "extraHeaders"],
);

// === Redirect Chain Tracking ===
// Maps original URL → final redirected URL so we can find captured headers
// after a redirect chain (e.g. pixeldrain.com → cdn-1.pixeldrain.com)
const redirectChains = new Map(); // Key: originalUrl, Value: { finalUrl, timestamp }
const REDIRECT_EXPIRY_MS = 120000; // 2 minutes

chrome.webRequest.onBeforeRedirect.addListener(
  (details) => {
    if (!details.url || !details.redirectUrl) return;

    // Find the chain start (walk back through existing chains)
    let originUrl = details.url;
    for (const [original, data] of redirectChains) {
      if (data.finalUrl === details.url) {
        originUrl = original;
        break;
      }
    }

    redirectChains.set(originUrl, {
      finalUrl: details.redirectUrl,
      timestamp: Date.now(),
    });


    // Cleanup old chains
    if (redirectChains.size > 500) {
      const now = Date.now();
      for (const [url, data] of redirectChains) {
        if (now - data.timestamp > REDIRECT_EXPIRY_MS) {
          redirectChains.delete(url);
        }
      }
    }
  },
  { urls: ["<all_urls>"] },
);

function cleanupExpiredHeaders() {
  const now = Date.now();
  for (const [url, data] of capturedHeaders) {
    if (now - data.timestamp > HEADER_EXPIRY_MS) {
      capturedHeaders.delete(url);
    }
  }
}

function getCapturedHeaders(url) {
  const data = capturedHeaders.get(url);
  if (!data) return null;

  // Check if expired
  if (Date.now() - data.timestamp > HEADER_EXPIRY_MS) {
    capturedHeaders.delete(url);
    return null;
  }

  return data.headers;
}

// Resolve headers by walking the redirect chain.
// Tries the exact URL first, then the final redirect target, then the original URL.
function resolveHeadersForUrl(url) {
  // Try direct match first
  let headers = getCapturedHeaders(url);
  if (headers) return headers;

  // Try looking up this URL's redirect chain to find the final URL's headers
  const chain = redirectChains.get(url);
  if (chain) {
    headers = getCapturedHeaders(chain.finalUrl);
    if (headers) return headers;
  }

  // Try reverse: maybe 'url' is a final URL, find the original
  for (const [original, data] of redirectChains) {
    if (data.finalUrl === url) {
      headers = getCapturedHeaders(original);
      if (headers) return headers;
    }
  }

  return null;
}

// Resolve the best download URL by checking for redirect chains.
// Returns the final URL if a redirect was tracked, otherwise the original.
function resolveDownloadUrl(url) {
  const chain = redirectChains.get(url);
  if (chain && Date.now() - chain.timestamp < REDIRECT_EXPIRY_MS) {
    return chain.finalUrl;
  }
  return url;
}

// Read cookies for a URL directly from the browser's cookie store.
// This is more reliable than onBeforeSendHeaders for download requests,
// which Chrome routes through the download manager without exposing headers
// to extensions via webRequest.
async function getCookiesAsHeader(url) {
  try {
    const cookies = await chrome.cookies.getAll({ url });
    if (!cookies || cookies.length === 0) return null;
    return cookies.map((c) => `${c.name}=${c.value}`).join("; ");
  } catch (e) {
    console.log("[Surge] Failed to read cookies:", e.message);
    return null;
  }
}

async function loadAuthToken() {
  if (cachedAuthToken !== null) {
    return cachedAuthToken;
  }
  const result = await chrome.storage.local.get(AUTH_TOKEN_KEY);
  cachedAuthToken = normalizeToken(result[AUTH_TOKEN_KEY]);
  return cachedAuthToken;
}

function normalizeToken(token) {
  if (typeof token !== "string") return "";
  return token.replace(/\s+/g, "");
}

async function setAuthToken(token) {
  const normalized = normalizeToken(token);
  for (let attempt = 1; attempt <= 3; attempt++) {
    await chrome.storage.local.set({ [AUTH_TOKEN_KEY]: normalized });
    const result = await chrome.storage.local.get(AUTH_TOKEN_KEY);
    if (normalizeToken(result[AUTH_TOKEN_KEY]) === normalized) {
      cachedAuthToken = normalized;
      return true;
    }
    await new Promise((resolve) => setTimeout(resolve, 50 * attempt));
  }
  return false;
}

async function authHeaders() {
  const token = await loadAuthToken();
  if (!token) return {};
  return { Authorization: `Bearer ${token}` };
}

// === Port Discovery / Server URL ===

async function getServerUrlConfig() {
  if (cachedServerUrl !== null) {
    return cachedServerUrl;
  }
  const result = await chrome.storage.local.get(SERVER_URL_KEY);
  cachedServerUrl = result[SERVER_URL_KEY] || "";
  return cachedServerUrl;
}

async function setServerUrlConfig(url) {
  const normalized = url ? url.trim().replace(/\/+$/, "") : "";
  await chrome.storage.local.set({ [SERVER_URL_KEY]: normalized });
  cachedServerUrl = normalized;
  return true;
}

chrome.storage.onChanged.addListener((changes, areaName) => {
  if (areaName !== "local") return;
  if (changes[SERVER_URL_KEY]) {
    cachedServerUrl = changes[SERVER_URL_KEY].newValue || "";
  }
  if (changes[AUTH_TOKEN_KEY]) {
    cachedAuthToken = normalizeToken(changes[AUTH_TOKEN_KEY].newValue);
  }
});

// Returns the base URL to use for API calls (e.g. "http://127.0.0.1:1700")
// or null if no server can be found
async function findSurgeUrl() {
  // 1. Try custom server URL if configured
  const customUrl = await getServerUrlConfig();
  if (customUrl) {
    try {
      const response = await fetch(`${customUrl}/health`, {
        method: "GET",
        signal: AbortSignal.timeout(1000),
      });
      if (response.ok) {
        const contentType = response.headers.get("content-type") || "";
        if (contentType.includes("application/json")) {
          const data = await response.json().catch(() => null);
          if (data && data.status === "ok") {
            isConnected = true;
            return customUrl;
          }
        }
      }
    } catch {}
    // If a custom URL is configured but unreachable, we don't fall back to localhost scanning
    // (User explicitly set it to a server, so we shouldn't guess what else to use)
    isConnected = false;
    return null;
  }

  // 2. Fall back to localhost port scanning
  // Try cached port first (with quick timeout)
  if (cachedPort) {
    try {
      const url = `http://127.0.0.1:${cachedPort}`;
      const response = await fetch(`${url}/health`, {
        method: "GET",
        signal: AbortSignal.timeout(300),
      });
      if (response.ok) {
        const contentType = response.headers.get("content-type") || "";
        if (contentType.includes("application/json")) {
          const data = await response.json().catch(() => null);
          if (data && data.status === "ok") {
            isConnected = true;
            return url;
          }
        }
      }
    } catch {}
    cachedPort = null;
  }

  // Scan for available port
  for (let port = DEFAULT_PORT; port < DEFAULT_PORT + MAX_PORT_SCAN; port++) {
    try {
      const url = `http://127.0.0.1:${port}`;
      const response = await fetch(`${url}/health`, {
        method: "GET",
        signal: AbortSignal.timeout(200),
      });
      if (response.ok) {
        const contentType = response.headers.get("content-type") || "";
        if (!contentType.includes("application/json")) {
          continue;
        }
        const data = await response.json().catch(() => null);
        if (!data || data.status !== "ok") {
          continue;
        }
        cachedPort = port;
        isConnected = true;
        console.log(`[Surge] Found server on port ${port}`);
        return url;
      }
    } catch {}
  }

  isConnected = false;
  return null;
}

async function checkSurgeHealth() {
  const now = Date.now();
  // Rate limit health checks to once per second
  if (now - lastHealthCheck < 1000) {
    return isConnected;
  }
  lastHealthCheck = now;

  const url = await findSurgeUrl();
  isConnected = url !== null;
  return isConnected;
}

// === Download List Fetching ===

async function fetchDownloadList() {
  const baseUrl = await findSurgeUrl();
  if (!baseUrl) {
    isConnected = false;
    return { list: [], authError: false };
  }

  try {
    const headers = await authHeaders();
    const response = await fetch(`${baseUrl}/list`, {
      method: "GET",
      headers,
      signal: AbortSignal.timeout(5000),
    });

    if (response.ok) {
      isConnected = true;
      const contentType = response.headers.get("content-type") || "";
      if (!contentType.includes("application/json")) {
        isConnected = false;
        return { list: [], authError: false };
      }
      let list;
      try {
        list = await response.json();
      } catch {
        isConnected = false;
        return { list: [], authError: false };
      }

      // Handle null or non-array response
      if (!Array.isArray(list)) {
        return { list: [], authError: false };
      }

      // Calculate ETA for each download
      const mapped = list.map((dl) => {
        let eta = null;
        if (dl.status === "downloading" && dl.speed > 0 && dl.total_size > 0) {
          const remaining = dl.total_size - dl.downloaded;
          // Speed is in MB/s, convert to bytes/s
          const speedBytes = dl.speed * MB;
          eta = Math.ceil(remaining / speedBytes);
        }
        return { ...dl, eta };
      });
      return { list: mapped, authError: false };
    } else {
      if (response.status === 401 || response.status === 403) {
        isConnected = true;
        return { list: [], authError: true };
      }
      isConnected = false;
      return { list: [], authError: false };
    }
  } catch (error) {
    console.error("[Surge] Error fetching downloads:", error);
  }

  return { list: [], authError: false };
}

async function fetchHistoryList() {
  const baseUrl = await findSurgeUrl();
  if (!baseUrl) {
    isConnected = false;
    return { list: [], authError: false };
  }

  try {
    const headers = await authHeaders();
    const response = await fetch(`${baseUrl}/history`, {
      method: "GET",
      headers,
      signal: AbortSignal.timeout(5000),
    });

    if (response.ok) {
      isConnected = true;
      const contentType = response.headers.get("content-type") || "";
      if (!contentType.includes("application/json")) {
        return { list: [], authError: false };
      }

      const list = await response.json().catch(() => []);
      if (!Array.isArray(list)) {
        return { list: [], authError: false };
      }

      // backend already returns history newest-first
      return { list: list.slice(0, 100), authError: false };
    }

    if (response.status === 401 || response.status === 403) {
      isConnected = true;
      return { list: [], authError: true };
    }

    return { list: [], authError: false };
  } catch (error) {
    console.error("[Surge] Error fetching history:", error);
    return { list: [], authError: false };
  }
}

async function openDownloadFile(id) {
  const baseUrl = await findSurgeUrl();
  if (!baseUrl) return { success: false, error: "Server not running" };

  try {
    const headers = await authHeaders();
    const response = await fetch(`${baseUrl}/open-file?id=${encodeURIComponent(id)}`, {
      method: "POST",
      headers,
      signal: AbortSignal.timeout(5000),
    });

    if (response.ok) return { success: true };

    const message = await response.text().catch(() => "Failed to open file");
    return { success: false, error: message };
  } catch (error) {
    return { success: false, error: error.message };
  }
}

async function openDownloadFolder(id) {
  const baseUrl = await findSurgeUrl();
  if (!baseUrl) return { success: false, error: "Server not running" };

  try {
    const headers = await authHeaders();
    const response = await fetch(`${baseUrl}/open-folder?id=${encodeURIComponent(id)}`, {
      method: "POST",
      headers,
      signal: AbortSignal.timeout(5000),
    });

    if (response.ok) return { success: true };

    const message = await response.text().catch(() => "Failed to open folder");
    return { success: false, error: message };
  } catch (error) {
    return { success: false, error: error.message };
  }
}

async function validateAuthToken() {
  const baseUrl = await findSurgeUrl();
  if (!baseUrl) {
    isConnected = false;
    return { ok: false, error: "no_server" };
  }
  try {
    const headers = await authHeaders();
    const response = await fetch(`${baseUrl}/list`, {
      method: "GET",
      headers,
      signal: AbortSignal.timeout(3000),
    });
    if (response.ok) {
      isConnected = true;
      return { ok: true };
    }
    return { ok: false, status: response.status };
  } catch (error) {
    return { ok: false, error: error.message };
  }
}

// === Download Sending ===

async function sendToSurge(url, filename, absolutePath) {
  const baseUrl = await findSurgeUrl();
  if (!baseUrl) {
    console.error("[Surge] No server found");
    return { success: false, error: "Server not running" };
  }

  try {
    const body = {
      url: url,
      filename: filename || "",
    };

    // Use absolute path directly if provided
    if (absolutePath) {
      body.path = absolutePath;
    }

    // Build headers to forward to Surge.
    // Primary: read cookies directly from browser storage (reliable for downloads).
    // Fallback: use any headers captured via onBeforeSendHeaders.
    const cookieString = await getCookiesAsHeader(url);
    const capturedHdrs = resolveHeadersForUrl(url);

    if (cookieString || capturedHdrs) {
      const merged = Object.assign({}, capturedHdrs || {});
      if (cookieString) {
        // Cookies from the store take precedence over captured headers
        merged["Cookie"] = cookieString;
      }
      body.headers = merged;
      console.log("[Surge] Forwarding headers to Surge (cookie:", !!cookieString, "captured:", !!capturedHdrs, ")");
    }

    const auth = await authHeaders();
    const response = await fetch(`${baseUrl}/download`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...auth,
      },
      body: JSON.stringify(body),
    });

    if (response.ok) {
      const data = await response.json();
      console.log("[Surge] Download queued:", data);
      return { success: true, data };
    } else if (response.status === 409) {
      const errorText = await response.text();
      let msg = "Download rejected: duplicate or approval required (headless mode)";
      try {
        const json = JSON.parse(errorText);
        if (json.message) msg = json.message;
      } catch (e) {}
      return { success: false, error: msg };
    } else {
      const error = await response.text();
      console.error(
        "[Surge] Failed to queue download:",
        response.status,
        error,
      );
      return { success: false, error };
    }
  } catch (error) {
    console.error("[Surge] Error sending to Surge:", error);
    return { success: false, error: error.message };
  }
}

// === Download Control ===

async function pauseDownload(id) {
  const baseUrl = await findSurgeUrl();
  if (!baseUrl) return false;

  try {
    const headers = await authHeaders();
    const response = await fetch(`${baseUrl}/pause?id=${id}`, {
      method: "POST",
      headers,
      signal: AbortSignal.timeout(5000),
    });
    return response.ok;
  } catch (error) {
    console.error("[Surge] Error pausing download:", error);
    return false;
  }
}

async function resumeDownload(id) {
  const baseUrl = await findSurgeUrl();
  if (!baseUrl) return false;

  try {
    const headers = await authHeaders();
    const response = await fetch(`${baseUrl}/resume?id=${id}`, {
      method: "POST",
      headers,
      signal: AbortSignal.timeout(5000),
    });
    return response.ok;
  } catch (error) {
    console.error("[Surge] Error resuming download:", error);
    return false;
  }
}

async function cancelDownload(id) {
  const baseUrl = await findSurgeUrl();
  if (!baseUrl) return false;

  try {
    const headers = await authHeaders();
    const response = await fetch(`${baseUrl}/delete?id=${id}`, {
      method: "DELETE",
      headers,
      signal: AbortSignal.timeout(5000),
    });
    return response.ok;
  } catch (error) {
    console.error("[Surge] Error canceling download:", error);
    return false;
  }
}

// === Interception State ===

async function isInterceptEnabled() {
  const result = await chrome.storage.local.get(INTERCEPT_ENABLED_KEY);
  return result[INTERCEPT_ENABLED_KEY] !== false;
}

// === Deduplication ===

// Check if URL is already being downloaded by Surge
async function isDuplicateDownload(url) {
  try {
    const { list } = await fetchDownloadList();
    if (Array.isArray(list) && list.length > 0) {
      const normalizedUrl = url.replace(/\/$/, ""); // Remove trailing slash
      for (const dl of list) {
        const normalizedDlUrl = (dl.url || "").replace(/\/$/, "");
        // Flag as duplicate if URL exists in Surge's download list (any status)
        if (normalizedDlUrl === normalizedUrl) {
          console.log(
            "[Surge] Duplicate download detected (already in Surge):",
            url,
          );
          return true;
        }
      }
    }
  } catch (e) {
    console.log("[Surge] Could not check Surge list for duplicates:", e);
  }

  return false;
}

function isFreshDownload(downloadItem) {
  // Must be in progress (not completed/interrupted from history)
  if (downloadItem.state && downloadItem.state !== "in_progress") {
    return false;
  }

  // Check start time
  if (!downloadItem.startTime) return true;

  const startTime = new Date(downloadItem.startTime).getTime();
  const now = Date.now();
  const diff = now - startTime;

  // If download started more than 30 seconds ago, likely history sync
  if (diff > 30000) {
    return false;
  }

  return true;
}

function shouldSkipUrl(url) {
  // Skip blob and data URLs - these exist only in browser memory and cannot
  // be forwarded to an external downloader
  if (url.startsWith("blob:") || url.startsWith("data:")) {
    console.log(
      "[Surge] Skipping blob/data URL (browser-memory-only, cannot be forwarded):",
      url.substring(0, 80),
    );
    return true;
  }

  // Skip chrome extension URLs
  if (url.startsWith("chrome-extension:") || url.startsWith("moz-extension:")) {
    return true;
  }

  return false;
}

// === Path Extraction ===

function extractPathInfo(downloadItem) {
  let filename = "";
  let directory = "";

  if (downloadItem.filename) {
    // downloadItem.filename contains the full path chosen by user
    // On Windows: C:\Users\Name\Downloads\file.zip
    // On macOS/Linux: /home/user/Downloads/file.zip

    const fullPath = downloadItem.filename;

    // Normalize separators and split
    const normalized = fullPath.replace(/\\/g, "/");
    const parts = normalized.split("/");

    filename = parts.pop() || "";

    if (parts.length > 0) {
      // Reconstruct directory path
      // On Windows, we need to preserve the drive letter
      if (/^[A-Za-z]:$/.test(parts[0])) {
        // Windows path with drive letter
        directory = parts.join("/");
      } else if (parts[0] === "") {
        // Unix absolute path (starts with /)
        directory = "/" + parts.slice(1).join("/");
      } else {
        directory = parts.join("/");
      }
    }
  }

  return { filename, directory };
}

// === Download Interception ===
// Uses onDeterminingFilename instead of onCreated to guarantee:
// 1. Filename is fully resolved (Content-Disposition parsed)
// 2. finalUrl is available (post-redirect)
// 3. Download is paused, giving us time to intercept cleanly

const processedIds = new Set();

// Must be registered synchronously at top level for MV3 service workers
chrome.downloads.onDeterminingFilename.addListener((downloadItem, suggest) => {
  // Return true to indicate we will call suggest() asynchronously
  handleDeterminingFilename(downloadItem, suggest);
  return true;
});

async function handleDeterminingFilename(downloadItem, suggest) {
  // Default: let browser proceed normally with whatever filename it resolved
  const allowBrowserDownload = () => {
    try {
      suggest();
    } catch (e) {
      // suggest() may throw if download was already cancelled
    }
  };

  try {
    // Prevent duplicate events for the same download ID
    if (processedIds.has(downloadItem.id)) {
      allowBrowserDownload();
      return;
    }

    const enabled = await isInterceptEnabled();
    if (!enabled) {
      console.log("[Surge] Interception disabled");
      allowBrowserDownload();
      return;
    }

    // Use finalUrl (post-redirect) if available, otherwise fall back to url
    const url = downloadItem.finalUrl || downloadItem.url;

    if (shouldSkipUrl(url)) {
      allowBrowserDownload();
      return;
    }

    if (!isFreshDownload(downloadItem)) {
      console.log("[Surge] Ignoring historical download");
      allowBrowserDownload();
      return;
    }

    // Fast pre-check using cached connection status to avoid a slow port scan
    // in the critical window before cancel(). If we know Surge is offline, bail.
    if (!isConnected) {
      const healthy = await checkSurgeHealth();
      if (!healthy) {
        console.log("[Surge] Server not running, allowing browser download");
        allowBrowserDownload();
        return;
      }
    }

    processedIds.add(downloadItem.id);
    setTimeout(() => processedIds.delete(downloadItem.id), 120000);

    // At this point, downloadItem.filename is the FINAL resolved filename
    // from Content-Disposition or URL, already parsed by Chrome.
    // This is the key advantage over onCreated.
    const filename = downloadItem.filename || "";

    console.log("[Surge] onDeterminingFilename:", {
      url,
      originalUrl: downloadItem.url,
      filename,
      mime: downloadItem.mime,
    });

    // Cancel the browser download NOW while it is still paused by onDeterminingFilename.
    // Must happen BEFORE any slow async operation to avoid the
    // "Download must be in progress" race condition.
    const cancelOk = await new Promise((resolve) => {
      chrome.downloads.cancel(downloadItem.id, () => {
        const err = chrome.runtime.lastError;
        if (err) {
          console.log("[Surge] Could not cancel download (already ended?):", err.message);
          resolve(false);
        } else {
          resolve(true);
        }
      });
    });

    // Release the filename determination lock regardless — must always be called.
    allowBrowserDownload();

    if (!cancelOk) {
      // Download already ended before we could take it — nothing more to do.
      processedIds.delete(downloadItem.id);
      return;
    }

    await new Promise((resolve) => {
      chrome.downloads.erase({ id: downloadItem.id }, () => {
        const err = chrome.runtime.lastError; // read to clear warning
        resolve();
      });
    });

    // Now confirm Surge is still reachable (cancel is done, timing no longer matters).
    const surgeRunning = await checkSurgeHealth();
    if (!surgeRunning) {
      console.log("[Surge] Server went offline after cancel — download lost");
      chrome.notifications.create({
        type: "basic",
        iconUrl: "icons/icon48.png",
        title: "Surge",
        message: "Download intercepted but Surge is not running. Please restart Surge and try again.",
      });
      return;
    }

    // Hand off to Surge with the correct filename and resolved headers
    await handleInterceptedDownload(url, downloadItem, filename);
  } catch (error) {
    console.error("[Surge] onDeterminingFilename error:", error);
    allowBrowserDownload();
  }
}

async function handleInterceptedDownload(url, downloadItem, filename) {
  // Check for duplicates
  if (await isDuplicateDownload(url)) {
    // Store pending duplicate and prompt user
    const pendingId = `dup_${++pendingDuplicateCounter}`;
    const displayName =
      filename || url.split("/").pop() || "Unknown file";

    pendingDuplicates.set(pendingId, {
      downloadItem,
      filename,
      directory: "",
      url: url,
      timestamp: Date.now(),
    });

    // Cleanup old pending duplicates (older than 60s)
    for (const [id, data] of pendingDuplicates) {
      if (Date.now() - data.timestamp > 60000) {
        pendingDuplicates.delete(id);
        updateBadge();
      }
    }

    // Update badge
    updateBadge();

    // Notify user via standard notification instead of popup (openPopup requires user gesture in MV3)
    chrome.notifications.create({
      type: "basic",
      iconUrl: "icons/icon48.png",
      title: "Surge / Duplicate",
      message: `Already downloading: ${displayName}`,
    });

    return;
  }

  // Send to Surge
  try {
    const result = await sendToSurge(url, filename, "");

    if (result.success) {
      if (result.data && result.data.status === "pending_approval") {
        chrome.notifications.create(`surge-confirm-${downloadItem.id}`, {
          type: "basic",
          iconUrl: "icons/icon48.png",
          title: "Surge - Confirmation Required",
          message: `Click to confirm download: ${filename || url.split("/").pop()}`,
          requireInteraction: true,
        });
        return; // Don't auto-open popup for pending interactions
      }

      // Show notification
      chrome.notifications.create({
        type: "basic",
        iconUrl: "icons/icon48.png",
        title: "Surge",
        message: `Download started: ${filename || url.split("/").pop()}`,
      });

      // Auto-open the popup to show download progress
      try {
        await chrome.action.openPopup();
      } catch (e) {
        // openPopup may fail if popup is already open or no user gesture
        console.log("[Surge] Could not auto-open popup:", e.message);
      }
    } else {
      // Failed to send to Surge - show error notification
      chrome.notifications.create({
        type: "basic",
        iconUrl: "icons/icon48.png",
        title: "Surge Error",
        message: `Failed to start download: ${result.error}`,
      });
    }
  } catch (error) {
    console.error("[Surge] Failed to send download to Surge:", error);
  }
}

// Handle notification clicks
chrome.notifications.onClicked.addListener((notificationId) => {
  if (notificationId.startsWith("surge-confirm-")) {
    // Attempt to open popup
    try {
      chrome.action.openPopup();
    } catch (e) {
      console.error("[Surge] Failed to open popup from notification:", e);
    }
    // Clear notification
    chrome.notifications.clear(notificationId);
  }
});

// === Message Handling ===

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  // Handle async responses
  (async () => {
    try {
      switch (message.type) {
        case "checkHealth": {
          const healthy = await checkSurgeHealth();
          sendResponse({ healthy });
          break;
        }

        case "getStatus": {
          const enabled = await isInterceptEnabled();
          sendResponse({ enabled });
          break;
        }
        case "getAuthToken": {
          const token = await loadAuthToken();
          const result = await chrome.storage.local.get(AUTH_VERIFIED_KEY);
          sendResponse({ token, verified: result[AUTH_VERIFIED_KEY] === true });
          break;
        }
        case "setAuthToken": {
          const persisted = await setAuthToken(message.token || "");
          if (!persisted) {
            sendResponse({ success: false, error: "Failed to persist auth token" });
            break;
          }
          // Reset verification on token change
          await chrome.storage.local.set({ [AUTH_VERIFIED_KEY]: false });
          sendResponse({ success: true });
          break;
        }

        case "getAuthVerified": {
          const result = await chrome.storage.local.get(AUTH_VERIFIED_KEY);
          sendResponse({ verified: result[AUTH_VERIFIED_KEY] === true });
          break;
        }

        case "setAuthVerified": {
          await chrome.storage.local.set({
            [AUTH_VERIFIED_KEY]: message.verified === true,
          });
          sendResponse({ success: true });
          break;
        }
        case "validateAuth": {
          const result = await validateAuthToken();
          sendResponse(result);
          break;
        }

        case "setStatus": {
          await chrome.storage.local.set({
            [INTERCEPT_ENABLED_KEY]: message.enabled,
          });
          sendResponse({ success: true });
          break;
        }

        case "getServerUrl": {
          const url = await getServerUrlConfig();
          sendResponse({ url });
          break;
        }

        case "setServerUrl": {
          await setServerUrlConfig(message.url);
          // Setting a new Server URL invalidates our health history and could change our connected status immediately
          lastHealthCheck = 0;
          sendResponse({ success: true });
          break;
        }

        case "getDownloads": {
          const { list, authError } = await fetchDownloadList();
          sendResponse({
            downloads: list,
            authError,
            connected: isConnected,
          });
          break;
        }

        case "getHistory": {
          const { list, authError } = await fetchHistoryList();
          sendResponse({ history: list, authError, connected: isConnected });
          break;
        }

        case "pauseDownload": {
          const success = await pauseDownload(message.id);
          sendResponse({ success });
          break;
        }

        case "resumeDownload": {
          const success = await resumeDownload(message.id);
          sendResponse({ success });
          break;
        }

        case "cancelDownload": {
          const success = await cancelDownload(message.id);
          sendResponse({ success });
          break;
        }

        case "openFile": {
          const result = await openDownloadFile(message.id);
          sendResponse(result);
          break;
        }

        case "openFolder": {
          const result = await openDownloadFolder(message.id);
          sendResponse(result);
          break;
        }

        case "confirmDuplicate": {
          // User confirmed duplicate download
          const pending = pendingDuplicates.get(message.id);
          console.log(
            "[Surge] confirmDuplicate called, pending:",
            pending ? "found" : "NOT FOUND",
            "id:",
            message.id,
          );
          if (pending) {
            pendingDuplicates.delete(message.id);
            updateBadge(); // Update badge

            console.log(
              "[Surge] Sending confirmed duplicate to Surge:",
              pending.url,
            );
            const result = await sendToSurge(
              pending.url,
              pending.filename,
              pending.directory,
            );
            console.log("[Surge] sendToSurge result:", result);

            if (result.success) {
              chrome.notifications.create({
                type: "basic",
                iconUrl: "icons/icon48.png",
                title: "Surge",
                message: `Download started: ${pending.filename || pending.url.split("/").pop()}`,
              });
            }

            // Check for next pending duplicate
            if (pendingDuplicates.size > 0) {
              const [nextId, nextData] = pendingDuplicates
                .entries()
                .next().value;
              const nextName =
                nextData.filename ||
                nextData.url.split("/").pop() ||
                "Unknown file";

              chrome.runtime
                .sendMessage({
                  type: "promptDuplicate",
                  id: nextId,
                  filename: nextName,
                })
                .catch(() => {});
            }

            sendResponse({ success: result.success });
          } else {
            sendResponse({
              success: false,
              error: "Pending download not found",
            });
          }
          break;
        }

        case "skipDuplicate": {
          // User skipped duplicate download
          const pending = pendingDuplicates.get(message.id);
          if (pending) {
            pendingDuplicates.delete(message.id);
            updateBadge(); // Update badge

            console.log(
              "[Surge] User skipped duplicate download:",
              pending.url,
            );

            // Check for next pending duplicate
            if (pendingDuplicates.size > 0) {
              const [nextId, nextData] = pendingDuplicates
                .entries()
                .next().value;
              const nextName =
                nextData.filename ||
                nextData.url.split("/").pop() ||
                "Unknown file";

              chrome.runtime
                .sendMessage({
                  type: "promptDuplicate",
                  id: nextId,
                  filename: nextName,
                })
                .catch(() => {});
            }
          }
          sendResponse({ success: true });
          break;
        }

        case "getPendingDuplicates": {
          const duplicates = [];
          for (const [id, data] of pendingDuplicates) {
            duplicates.push({
              id,
              filename:
                data.filename || data.url.split("/").pop() || "Unknown file",
              url: data.url,
            });
          }
          sendResponse({ duplicates });
          break;
        }

        default:
          sendResponse({ error: "Unknown message type" });
      }
    } catch (error) {
      console.error("[Surge] Message handler error:", error);
      sendResponse({ error: error.message });
    }
  })();

  return true; // Keep channel open for async response
});

// === Initialization ===

async function initialize() {
  console.log("[Surge] Extension initializing...");
  await loadAuthToken();
  await checkSurgeHealth();
  console.log("[Surge] Extension loaded");
}

chrome.runtime.onInstalled.addListener(() => {
  initialize().catch((error) =>
    console.error("[Surge] Initialization on install failed:", error),
  );
});

chrome.runtime.onStartup.addListener(() => {
  initialize().catch((error) =>
    console.error("[Surge] Initialization on startup failed:", error),
  );
});

initialize();
