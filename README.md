# laraserve

Serve any Laravel app over **HTTPS by default** on a machine with **no PHP, Composer or web server installed**.
A small Go executable downloads [FrankenPHP](https://frankenphp.dev) (PHP + Caddy),
Composer, MySQL and phpMyAdmin **on the client, only when needed**, and caches them
in `~/.laraserve/runtimes/`. Later runs can serve prepared apps offline.

```
laraserve new my-app                    # create a Laravel app (needs internet or Composer cache)
laraserve serve                          # https://localhost:8443
laraserve serve --domain testapp.test    # https://testapp.test (443 if allowed, else :8443)
laraserve serve my-app                   # detects MySQL settings and opens local phpMyAdmin
laraserve serve my-app --mysql           # force private MySQL for an app using another database
laraserve serve --http                   # http://localhost:8080  (no CA, no trust prompt, no host change)
laraserve serve ./other-app --copy       # run on a copy, original untouched
laraserve serve app.laraserve            # serve an offline bundle
laraserve pack                           # bundle app + vendor/ + built assets (run where you have internet)
laraserve trust | untrust                # install / remove the local CA
laraserve clean | uninstall              # reverse everything, driven by ~/.laraserve/manifest.json
laraserve doctor
```
