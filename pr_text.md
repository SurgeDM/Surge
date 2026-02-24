Title: "⚡ Bolt: Eliminating CDN Range Drops and Worker Stalls"

Description:

- 💡 What: Fixed the HTTP client configuration to preserve `Range` headers during redirects, and updated worker keepalaives so they don't false-positive timeout.
- 🎯 Why: `CheckRedirect` was dropping `Range` on 301/302 redirects, silently breaking chunking for almost all CDNs. The Health monitor was also incorrectly checking for 5-second connection 'stalls' based on disk flush times rather than streaming TCP activity.
- 📊 Impact: CDNs that redirect will now properly parallelize, drastically speeding up overall download rates on modern infrastrucutres. Worker connections will no longer constantly drop when writing their 512KB buffers to disk, preventing thrashing and saving partial progress.
